package server_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/han/runic/internal/client"
	"github.com/han/runic/internal/config"
	"github.com/han/runic/internal/protocol"
	"github.com/han/runic/internal/server"
)

// startDaemon brings up a daemon on a private socket with isolated config and
// state directories, and returns nothing: every client call in these tests goes
// through RUNIC_SOCKET like a real `ru` invocation would.
func startDaemon(t *testing.T) {
	t.Helper()
	sock := shortSocketPath(t)
	t.Setenv("RUNIC_SOCKET", sock)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("SHELL", "/bin/sh")

	srv, err := server.New(config.Load())
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go srv.Run(ctx)
	t.Cleanup(func() { srv.Shutdown(); cancel() })
	waitForSocket(t, sock)
}

// The full `ru mark` path over the wire: a pane's state shows up in both
// `ru term ls` and the tree poll the TUI renders from.
func TestMarkOverTheWire(t *testing.T) {
	startDaemon(t)

	if err := client.SessionCreate("agent"); err != nil {
		t.Fatalf("SessionCreate: %v", err)
	}
	pane, err := client.OpenTerminal("agent", 80, 24)
	if err != nil {
		t.Fatalf("OpenTerminal: %v", err)
	}

	if err := client.Mark("agent", pane, protocol.AttentionWaiting, "which schema?"); err != nil {
		t.Fatalf("Mark: %v", err)
	}

	terms, err := client.ListTerminals()
	if err != nil {
		t.Fatalf("ListTerminals: %v", err)
	}
	var found bool
	for _, term := range terms {
		if term.Session == "agent" && term.Pane == pane {
			found = true
			if term.Attention != protocol.AttentionWaiting {
				t.Fatalf("pane attention = %v, want waiting", term.Attention)
			}
			if term.Note != "which schema?" {
				t.Fatalf("pane note = %q", term.Note)
			}
			if term.Since.IsZero() {
				t.Fatal("pane Since not set")
			}
		}
	}
	if !found {
		t.Fatalf("pane %s not in ListTerminals: %+v", pane, terms)
	}

	// The same state must reach the TUI through the tree poll, rolled up to the
	// session — that is the only call the management view makes.
	data, err := client.TreeData()
	if err != nil {
		t.Fatalf("TreeData: %v", err)
	}
	for _, s := range data.Sessions {
		if s.Name != "agent" {
			continue
		}
		if s.Attention != protocol.AttentionWaiting || s.AttentionNote != "which schema?" {
			t.Fatalf("session rollup = %v/%q, want waiting/\"which schema?\"", s.Attention, s.AttentionNote)
		}
		if !s.Attention.NeedsYou() {
			t.Fatal("waiting must count as needing the user")
		}
		return
	}
	t.Fatalf("session \"agent\" missing from the tree: %+v", data.Sessions)
}

// Marking a session with no live pane is an error rather than a silent no-op:
// a mark nothing will ever display is a misconfiguration worth reporting.
func TestMarkWithoutPaneErrors(t *testing.T) {
	startDaemon(t)
	if err := client.Mark("ghost", "", protocol.AttentionWaiting, ""); err == nil {
		t.Fatal("expected an error marking a session with no live pane")
	}
}

// A worktree session, end to end: the daemon creates the checkout, its panes
// actually start there, and deleting the session removes the directory.
func TestWorktreeSessionOverTheWire(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	startDaemon(t)
	repo := initTestRepo(t)

	if err := client.SessionCreateWorktree("agent-1", true, repo, ""); err != nil {
		t.Fatalf("SessionCreateWorktree: %v", err)
	}

	data, err := client.TreeData()
	if err != nil {
		t.Fatalf("TreeData: %v", err)
	}
	var wtPath, branch string
	for _, s := range data.Sessions {
		if s.Name == "agent-1" {
			wtPath, branch = s.Worktree, s.Branch
		}
	}
	if wtPath == "" {
		t.Fatalf("session has no worktree in the tree: %+v", data.Sessions)
	}
	if branch != "agent-1" {
		t.Fatalf("branch = %q, want agent-1", branch)
	}
	if st, err := os.Stat(wtPath); err != nil || !st.IsDir() {
		t.Fatalf("worktree directory %s missing: %v", wtPath, err)
	}

	// The pane must really open in the worktree — that is the whole point.
	pane, err := client.OpenTerminal("agent-1", 80, 24)
	if err != nil {
		t.Fatalf("OpenTerminal: %v", err)
	}
	c, err := client.AttachTerminal("agent-1", pane, 80, 24)
	if err != nil {
		t.Fatalf("AttachTerminal: %v", err)
	}
	_ = c.Send(&protocol.Msg{
		Type:    protocol.MsgTerminalInput,
		Payload: protocol.PayloadTerminalData{Data: []byte("pwd\r")},
	})
	// Compare on the base name: macOS reports /private/var for /var.
	if !recvUntil(t, c, []byte(filepath.Base(wtPath)), 3*time.Second) {
		t.Fatalf("pane did not start in the worktree %s", wtPath)
	}
	c.Close()

	// Deleting the session removes the worktree but keeps the branch.
	if err := client.SessionDelete("agent-1"); err != nil {
		t.Fatalf("SessionDelete: %v", err)
	}
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Fatalf("worktree %s survived session deletion: %v", wtPath, err)
	}
	if err := exec.Command("git", "-C", repo, "rev-parse", "--verify", "agent-1").Run(); err != nil {
		t.Fatal("branch was deleted with the worktree; its commits must be kept")
	}
}

// An ordinary session must be unaffected by the worktree machinery.
func TestPlainSessionHasNoWorktree(t *testing.T) {
	startDaemon(t)
	if err := client.SessionCreate("plain"); err != nil {
		t.Fatalf("SessionCreate: %v", err)
	}
	data, err := client.TreeData()
	if err != nil {
		t.Fatalf("TreeData: %v", err)
	}
	for _, s := range data.Sessions {
		if s.Name == "plain" && (s.Worktree != "" || s.Branch != "") {
			t.Fatalf("plain session reports worktree %q branch %q", s.Worktree, s.Branch)
		}
	}
}

// Asking for a worktree outside a repository fails, and must not leave a
// half-created session behind.
func TestWorktreeSessionRollsBackOnFailure(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	startDaemon(t)

	if err := client.SessionCreateWorktree("doomed", true, t.TempDir(), ""); err == nil {
		t.Fatal("expected an error creating a worktree outside a git repository")
	}
	sessions, err := client.SessionList()
	if err != nil {
		t.Fatalf("SessionList: %v", err)
	}
	for _, s := range sessions {
		if s == "doomed" {
			t.Fatal("failed worktree create left the session behind")
		}
	}
}

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "runicrepo")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "test"},
		{"commit", "-q", "--allow-empty", "-m", "root"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	return dir
}

// An agent's uncommitted work must survive the deletion of its session. The
// daemon refuses, says what is at risk, and leaves both the session and the
// checkout alone; only an explicit discard throws the work away.
func TestDeleteWorktreeSessionProtectsUncommittedWork(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	startDaemon(t)
	repo := initTestRepo(t)

	if err := client.SessionCreateWorktree("agent-wip", true, repo, ""); err != nil {
		t.Fatalf("SessionCreateWorktree: %v", err)
	}
	wtPath := worktreePathOf(t, "agent-wip")

	// Stand in for the agent: a file it has not committed yet.
	scratch := filepath.Join(wtPath, "half-done.go")
	if err := os.WriteFile(scratch, []byte("package main\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	err := client.SessionDelete("agent-wip")
	var dirty *client.DirtyWorktreeError
	if !errors.As(err, &dirty) {
		t.Fatalf("SessionDelete = %v, want a *client.DirtyWorktreeError", err)
	}
	if !errors.Is(err, client.ErrDirtyWorktree) {
		t.Errorf("error should satisfy errors.Is(err, ErrDirtyWorktree), got %v", err)
	}
	if !strings.Contains(dirty.Detail, "half-done.go") {
		t.Errorf("detail should name the file at risk, got %q", dirty.Detail)
	}
	if _, statErr := os.Stat(scratch); statErr != nil {
		t.Fatalf("uncommitted file was deleted by a refused delete: %v", statErr)
	}
	if worktreePathOf(t, "agent-wip") == "" {
		t.Fatal("session was deleted even though its worktree was kept")
	}

	// Asking for the loss by name goes through.
	if err := client.SessionDeleteDiscard("agent-wip", true); err != nil {
		t.Fatalf("SessionDeleteDiscard: %v", err)
	}
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Fatalf("worktree %s survived a discarding delete: %v", wtPath, err)
	}
}

// worktreePathOf returns a session's worktree directory as the tree poll
// reports it, or "" when the session is gone or has none.
func worktreePathOf(t *testing.T, session string) string {
	t.Helper()
	data, err := client.TreeData()
	if err != nil {
		t.Fatalf("TreeData: %v", err)
	}
	for _, s := range data.Sessions {
		if s.Name == session {
			return s.Worktree
		}
	}
	return ""
}
