package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/han/runic/internal/protocol"
)

func TestParseGitHead(t *testing.T) {
	cases := map[string]string{
		"ref: refs/heads/main":              "main",
		"ref: refs/heads/feature/ui_update": "feature/ui_update",
		"ref: refs/heads/fix/a/b":           "fix/a/b",
		"0123456789abcdef0123456789abcdef":  "0123456",
		"":                                  "",
	}
	for head, want := range cases {
		if got := parseGitHead(head); got != want {
			t.Errorf("parseGitHead(%q) = %q, want %q", head, got, want)
		}
	}
}

func TestGitBranchInDir(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/feature/ui_update\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Resolves from a nested subdirectory by walking up.
	sub := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := gitBranchIn(sub); got != "feature/ui_update" {
		t.Fatalf("gitBranchIn = %q, want feature/ui_update", got)
	}

	// Not a git repo.
	if got := gitBranchIn(t.TempDir()); got != "" {
		t.Fatalf("expected empty branch outside a repo, got %q", got)
	}
}

func TestGitBranchWorktreePointer(t *testing.T) {
	// A worktree's .git is a file pointing at the real gitdir.
	dir := t.TempDir()
	realGitDir := filepath.Join(dir, "realgit")
	if err := os.MkdirAll(realGitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realGitDir, "HEAD"), []byte("ref: refs/heads/wt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(dir, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, ".git"), []byte("gitdir: "+realGitDir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := gitBranchIn(work); got != "wt" {
		t.Fatalf("gitBranchIn worktree = %q, want wt", got)
	}
}

// The ^B n/p hint has to appear whenever the chord has somewhere to go, which
// is every session in the daemon's catalog — not only the ones this client has
// already opened. Gating it on the open layouts hid the hint while the tab
// strip was plainly offering three sessions to switch to.
func TestSessionChordHintFollowsTheCatalog(t *testing.T) {
	m := tabsSessionModel()
	m.jobs.tree.focus = false
	// Wide enough that renderStatusBar keeps the right-hand key caps at all —
	// it drops them wholesale rather than truncating.
	m.width = 200
	if !strings.Contains(stripAnsi(m.renderFooter()), "^B n/p") {
		t.Error("four sessions in the catalog should advertise the chord")
	}

	// Only one session is open, but the catalog still holds four: the chord
	// works, so the hint stays.
	m.layouts = map[string]*paneNode{"agent-2": m.layouts["agent-2"]}
	if !strings.Contains(stripAnsi(m.renderFooter()), "^B n/p") {
		t.Error("unopened sessions are still reachable; the hint must stay")
	}

	// A genuinely single-session catalog has nowhere to go.
	m.nodes = buildTree([]string{"default"},
		[]protocol.SessionInfo{{Name: "agent-2", Group: "default"}}, nil)
	if strings.Contains(stripAnsi(m.renderFooter()), "^B n/p") {
		t.Error("one session should not advertise a chord that does nothing")
	}
}

func TestModeName(t *testing.T) {
	if (model{focus: paneTerm}).modeName() != "INSERT" {
		t.Error("terminal focus should be INSERT")
	}
	if (model{focus: paneTerm, inputMode: inputCommand}).modeName() != "COMMAND" {
		t.Error("command input should be COMMAND")
	}
	if (model{focus: paneTerm, inputMode: inputCreate}).modeName() != "COMMAND" {
		t.Error("text input should be COMMAND")
	}
}
