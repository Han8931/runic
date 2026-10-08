package server

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/han/runic/internal/protocol"
)

// initRepo makes a throwaway git repo with one commit, so `git worktree add`
// has a HEAD to branch from.
func initRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "test"},
		{"commit", "-q", "--allow-empty", "-m", "root"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	return dir
}

func TestAddAndRemoveWorktree(t *testing.T) {
	repo := initRepo(t)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	wt, err := addWorktree("agent-1", repo, "")
	if err != nil {
		t.Fatalf("addWorktree: %v", err)
	}
	if wt.Branch != "agent-1" {
		t.Fatalf("branch = %q, want the session name", wt.Branch)
	}
	if st, err := os.Stat(wt.Path); err != nil || !st.IsDir() {
		t.Fatalf("worktree directory %s not created: %v", wt.Path, err)
	}
	// The worktree must be a real checkout of the new branch.
	out, err := exec.Command("git", "-C", wt.Path, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		t.Fatalf("rev-parse in worktree: %v", err)
	}
	if got := string(out); got != "agent-1\n" {
		t.Fatalf("worktree HEAD = %q, want agent-1", got)
	}

	if err := removeWorktree(wt, false); err != nil {
		t.Fatalf("removeWorktree: %v", err)
	}
	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Fatalf("worktree directory still present after removal: %v", err)
	}
	// The branch survives: its commits are the point of the exercise.
	if err := exec.Command("git", "-C", repo, "rev-parse", "--verify", "agent-1").Run(); err != nil {
		t.Fatal("branch was deleted along with the worktree; it must be kept")
	}
}

// Uncommitted work is not recoverable from the branch, so a dirty worktree is
// kept and the removal refused — unless the loss is asked for explicitly.
func TestRemoveWorktreeKeepsDirtyTree(t *testing.T) {
	repo := initRepo(t)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	wt, err := addWorktree("dirty", repo, "")
	if err != nil {
		t.Fatalf("addWorktree: %v", err)
	}
	scratch := filepath.Join(wt.Path, "scratch.txt")
	if err := os.WriteFile(scratch, []byte("wip"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	err = removeWorktree(wt, false)
	var dirty *DirtyWorktreeError
	if !errors.As(err, &dirty) {
		t.Fatalf("removeWorktree on a dirty worktree = %v, want a *DirtyWorktreeError", err)
	}
	if !strings.Contains(err.Error(), "scratch.txt") {
		t.Errorf("error should name the work at risk, got %q", err)
	}
	if !strings.HasPrefix(err.Error(), protocol.DirtyWorktreePrefix) {
		t.Errorf("error must carry the wire prefix clients match on, got %q", err)
	}
	if _, statErr := os.Stat(scratch); statErr != nil {
		t.Fatalf("uncommitted file was deleted by a refused removal: %v", statErr)
	}

	// An explicit discard goes through.
	if err := removeWorktree(wt, true); err != nil {
		t.Fatalf("removeWorktree with discard: %v", err)
	}
	if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
		t.Fatal("discarded worktree was not removed")
	}
}

// Work that is already committed on the branch is safe without the worktree, so
// a clean-but-ahead worktree removes without ceremony.
func TestRemoveWorktreeWithCommittedWork(t *testing.T) {
	repo := initRepo(t)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	wt, err := addWorktree("committed", repo, "")
	if err != nil {
		t.Fatalf("addWorktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wt.Path, "done.txt"), []byte("work"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", "work"}} {
		cmd := exec.Command("git", append([]string{"-C", wt.Path}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	if err := removeWorktree(wt, false); err != nil {
		t.Fatalf("removeWorktree on a clean worktree: %v", err)
	}
}

// A worktree whose directory is already gone must not block the cleanup it no
// longer needs.
func TestRemoveWorktreeMissingDirectory(t *testing.T) {
	repo := initRepo(t)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	wt, err := addWorktree("vanished", repo, "")
	if err != nil {
		t.Fatalf("addWorktree: %v", err)
	}
	if err := os.RemoveAll(wt.Path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := removeWorktree(wt, false); err != nil {
		t.Fatalf("removeWorktree on a missing directory: %v", err)
	}
}

func TestAddWorktreeRejectsNonRepo(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if _, err := addWorktree("s", t.TempDir(), ""); err == nil {
		t.Fatal("expected an error outside a git repository")
	}
}

func TestAddWorktreeCustomBranch(t *testing.T) {
	repo := initRepo(t)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	wt, err := addWorktree("s", repo, "feature/x")
	if err != nil {
		t.Fatalf("addWorktree: %v", err)
	}
	defer removeWorktree(wt, true)
	// '/' is not in the safe set, so the branch is sanitized rather than
	// creating a nested ref the directory name cannot mirror.
	if wt.Branch != "feature-x" {
		t.Fatalf("branch = %q, want feature-x", wt.Branch)
	}
}

// A hand-edited snapshot must not be able to aim the daemon's rm at an
// arbitrary directory.
func TestRemoveWorktreeRefusesOutsidePaths(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	victim := t.TempDir()
	err := removeWorktree(SessionWorktree{Path: victim}, true)
	if err == nil {
		t.Fatal("expected a refusal for a path outside the worktree directory")
	}
	if _, statErr := os.Stat(victim); statErr != nil {
		t.Fatalf("refused path was deleted anyway: %v", statErr)
	}
}

func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		"agent-1":       "agent-1",
		"fix: the bug":  "fix-the-bug",
		"../../etc":     "etc",
		"feature/x":     "feature-x",
		"":              "session",
		"...":           "session",
		"Déjà vu":       "D-j-vu",
		"ok_name.v2":    "ok_name.v2",
		"--leading--":   "leading",
		"with\nnewline": "with-newline",
	}
	for in, want := range cases {
		if got := sanitizeName(in); got != want {
			t.Errorf("sanitizeName(%q) = %q, want %q", in, got, want)
		}
	}
}

// Deleting a worktree session must remove its directory; deleting an ordinary
// one must not go looking for anything.
func TestSessionWorktreeBookkeeping(t *testing.T) {
	q := NewJobQueue()
	if !q.CreateSession("s") {
		t.Fatal("CreateSession")
	}
	q.SetSessionWorktree("s", SessionWorktree{Path: "/tmp/x", Branch: "b", Repo: "/repo"})

	if dir := q.SessionDir("s"); dir != "/tmp/x" {
		t.Fatalf("SessionDir = %q, want /tmp/x", dir)
	}
	if dir := q.SessionDir("default"); dir != "" {
		t.Fatalf("SessionDir for a plain session = %q, want empty", dir)
	}

	// A rename carries the worktree over rather than orphaning it.
	if !q.RenameSession("s", "s2") {
		t.Fatal("RenameSession")
	}
	if _, ok := q.SessionWorktreeFor("s"); ok {
		t.Fatal("worktree still recorded under the old name")
	}
	wt, ok := q.SessionWorktreeFor("s2")
	if !ok || wt.Branch != "b" {
		t.Fatalf("worktree not carried to the new name: %+v, ok=%v", wt, ok)
	}

	if ok, reason := q.DeleteSession("s2"); !ok {
		t.Fatalf("DeleteSession: %s", reason)
	}
	if _, ok := q.SessionWorktreeFor("s2"); ok {
		t.Fatal("worktree record survived session deletion")
	}
}

// Worktree records must survive a daemon restart, and a snapshot written before
// worktrees existed must still load.
func TestWorktreePersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.json")

	q := NewJobQueue()
	q.CreateSession("s")
	q.SetSessionWorktree("s", SessionWorktree{Path: "/tmp/wt", Branch: "b", Repo: "/repo"})
	if err := q.save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := loadJobQueue(path)
	if err != nil {
		t.Fatalf("loadJobQueue: %v", err)
	}
	wt, ok := loaded.SessionWorktreeFor("s")
	if !ok || wt.Path != "/tmp/wt" || wt.Branch != "b" || wt.Repo != "/repo" {
		t.Fatalf("worktree not restored: %+v, ok=%v", wt, ok)
	}

	// A snapshot from before this feature has no "worktrees" key at all.
	old := filepath.Join(t.TempDir(), "old.json")
	if err := os.WriteFile(old, []byte(`{"version":1,"next_id":0,"last_id":0,"jobs":[],"sessions":{"default":"default"},"groups":{"default":true}}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	legacy, err := loadJobQueue(old)
	if err != nil {
		t.Fatalf("loadJobQueue on a pre-worktree snapshot: %v", err)
	}
	if dir := legacy.SessionDir("default"); dir != "" {
		t.Fatalf("SessionDir = %q on a legacy snapshot, want empty", dir)
	}
}
