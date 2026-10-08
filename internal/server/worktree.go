package server

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/han/runic/internal/config"
	"github.com/han/runic/internal/protocol"
)

// Worktree-per-session isolation: a session can own a fresh `git worktree` on
// its own branch, so two agents working the same repo never see each other's
// edits. Groups become projects, sessions become agent runs.
//
// The worktree directory is always chosen by runic, under its own state dir —
// never a path the caller named. That is what makes removing it on session
// delete safe: the daemon only ever deletes directories it created.

// SessionWorktree records the worktree a session owns.
type SessionWorktree struct {
	Path   string `json:"path"`   // the worktree directory (under config.WorktreeDir())
	Branch string `json:"branch"` // the branch checked out in it
	Repo   string `json:"repo"`   // the repository it was created from
}

// safeName is the subset of characters allowed in a generated branch or
// directory name. Everything else collapses to '-' so a session called
// "fix: ../../etc" cannot escape the worktree directory or confuse git.
var safeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func sanitizeName(s string) string {
	out := strings.Trim(safeName.ReplaceAllString(s, "-"), "-._")
	if out == "" {
		return "session"
	}
	return out
}

// repoRoot resolves dir to the top level of its git work tree.
func repoRoot(dir string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("no repository directory given")
	}
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s is not inside a git repository", dir)
	}
	return strings.TrimSpace(string(out)), nil
}

// addWorktree creates a worktree for a session: a new branch off the repo's
// current HEAD, checked out in a runic-owned directory. branch may be empty, in
// which case it is derived from the session name.
func addWorktree(session, repoDir, branch string) (SessionWorktree, error) {
	root, err := repoRoot(repoDir)
	if err != nil {
		return SessionWorktree{}, err
	}
	base := config.WorktreeDir()
	if base == "" {
		return SessionWorktree{}, fmt.Errorf("cannot locate a state directory for worktrees")
	}
	if err := os.MkdirAll(base, 0o700); err != nil {
		return SessionWorktree{}, fmt.Errorf("create worktree directory: %w", err)
	}

	if branch == "" {
		branch = sanitizeName(session)
	} else {
		branch = sanitizeName(branch)
	}
	path := filepath.Join(base, sanitizeName(session))
	if _, err := os.Stat(path); err == nil {
		return SessionWorktree{}, fmt.Errorf("worktree directory %s already exists", path)
	}

	out, err := exec.Command("git", "-C", root, "worktree", "add", "-b", branch, path).CombinedOutput()
	if err != nil {
		return SessionWorktree{}, fmt.Errorf("git worktree add: %s", strings.TrimSpace(string(out)))
	}
	return SessionWorktree{Path: path, Branch: branch, Repo: root}, nil
}

// DirtyWorktreeError reports that a worktree still holds work `git worktree
// remove --force` would destroy: uncommitted edits to tracked files, or
// untracked files that are in no commit at all. Keeping the branch saves
// commits, but nothing saves these — so removal refuses by default and the
// caller has to ask for the loss explicitly.
type DirtyWorktreeError struct {
	Path    string
	Entries []string // `git status --porcelain` lines, as reported
}

func (e *DirtyWorktreeError) Error() string {
	// The wire prefix lets the client recognize this specific refusal and offer
	// the discard, rather than string-matching a free-form message.
	what := fmt.Sprintf("%d uncommitted change", len(e.Entries))
	if len(e.Entries) != 1 {
		what += "s"
	}
	return fmt.Sprintf("%s%s in %s: %s", protocol.DirtyWorktreePrefix, what, e.Path, strings.Join(e.summary(), ", "))
}

// summary names at most three changed paths, so the message stays one line.
func (e *DirtyWorktreeError) summary() []string {
	const show = 3
	out := make([]string, 0, show+1)
	for i, entry := range e.Entries {
		if i == show {
			out = append(out, fmt.Sprintf("… and %d more", len(e.Entries)-show))
			break
		}
		// Porcelain lines are "XY path"; the status letters are noise here.
		out = append(out, strings.TrimSpace(entry[min(len(entry), 3):]))
	}
	return out
}

// worktreeChanges lists the worktree's uncommitted changes, untracked files
// included. An unreadable worktree (directory gone, repo gone) reports no
// changes: there is nothing left to protect, and refusing to clean up after a
// directory that does not exist would strand the session.
func worktreeChanges(path string) []string {
	out, err := exec.Command("git", "-C", path, "status", "--porcelain", "--untracked-files=all").Output()
	if err != nil {
		return nil
	}
	var entries []string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) != "" {
			entries = append(entries, line)
		}
	}
	return entries
}

// removeWorktree tears a session's worktree down, pruning the repo's
// administrative record afterwards. The branch is deliberately left behind: the
// work on it is the whole point, and deleting it would discard commits the user
// may not have pushed.
//
// Uncommitted and untracked work is not recoverable from the branch, so a dirty
// worktree is kept where it is and reported as a *DirtyWorktreeError unless
// discard says otherwise. The directory outliving its session is the lesser
// evil: it is still a normal checkout the user can salvage from (`git -C <path>
// status`), whereas a forced removal is final.
func removeWorktree(wt SessionWorktree, discard bool) error {
	if wt.Path == "" {
		return nil
	}
	// Refuse to touch anything outside the directory runic owns, even if a
	// hand-edited snapshot claims otherwise.
	base := config.WorktreeDir()
	if base == "" || !isWithin(base, wt.Path) {
		return fmt.Errorf("refusing to remove %s: outside %s", wt.Path, base)
	}

	if !discard {
		if entries := worktreeChanges(wt.Path); len(entries) > 0 {
			return &DirtyWorktreeError{Path: wt.Path, Entries: entries}
		}
	}

	if wt.Repo != "" {
		out, err := exec.Command("git", "-C", wt.Repo, "worktree", "remove", "--force", wt.Path).CombinedOutput()
		if err != nil {
			// The repo may be gone, or the worktree already removed by hand.
			// Fall through to deleting the directory, then prune the record.
			if _, statErr := os.Stat(wt.Path); statErr == nil {
				if rmErr := os.RemoveAll(wt.Path); rmErr != nil {
					return fmt.Errorf("git worktree remove: %s", strings.TrimSpace(string(out)))
				}
			}
			_ = exec.Command("git", "-C", wt.Repo, "worktree", "prune").Run()
			return nil
		}
		return nil
	}
	return os.RemoveAll(wt.Path)
}

// isWithin reports whether path is base or sits underneath it, comparing
// cleaned absolute paths so ".." cannot sneak out.
func isWithin(base, path string) bool {
	absBase, err := filepath.Abs(base)
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absBase, absPath)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
