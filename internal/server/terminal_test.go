package server

import (
	"bytes"
	"testing"
	"time"

	"github.com/han/runic/internal/protocol"
)

func TestTerminalManagerReattachReplaysBacklog(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	m := NewTerminalManager()
	term, err := m.GetOrCreate("test", "main", 80, 24)
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	defer m.Kill("test", "main")

	ch, _ := term.Attach()
	term.Write([]byte("printf runic-attach-ok\\n\r"))
	if !readUntil(t, ch, []byte("runic-attach-ok"), 2*time.Second) {
		t.Fatal("did not see command output")
	}
	term.Detach(ch)

	_, backlog := term.Attach()
	if !bytes.Contains(backlog, []byte("runic-attach-ok")) {
		t.Fatalf("expected backlog to contain command output, got %q", backlog)
	}
}

func TestTerminalOpenAssignsUniqueNames(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	m := NewTerminalManager()
	a, err := m.Open("s", 80, 24)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	b, err := m.Open("s", 80, 24)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer m.KillAll()
	if a == b {
		t.Fatalf("expected unique pane names, got %q twice", a)
	}
}

func TestTerminalLayoutPersistAndReap(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	m := NewTerminalManager()
	a, _ := m.Open("s", 80, 24)
	b, _ := m.Open("s", 80, 24)
	defer m.KillAll()

	blob := []byte("layout-blob")
	// Keep only pane a; b must be reaped.
	m.SetLayout("s", blob, []string{a})

	gotBlob, alive := m.ListLayout("s")
	if string(gotBlob) != "layout-blob" {
		t.Fatalf("layout blob = %q, want layout-blob", gotBlob)
	}
	if len(alive) != 1 || alive[0] != a {
		t.Fatalf("alive = %v, want [%s]", alive, a)
	}
	if _, ok := m.sessions[terminalKey("s", b)]; ok {
		t.Fatalf("pane %s should have been reaped", b)
	}
}

func TestTerminalListAll(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	m := NewTerminalManager()
	a, _ := m.Open("s1", 80, 24)
	b, _ := m.Open("s2", 80, 24)
	defer m.KillAll()

	all := m.ListAll()
	if len(all) != 2 {
		t.Fatalf("ListAll = %d entries, want 2", len(all))
	}
	seen := map[string]bool{}
	for _, ti := range all {
		seen[ti.Session+"/"+ti.Pane] = true
	}
	if !seen["s1/"+a] || !seen["s2/"+b] {
		t.Fatalf("ListAll missing expected panes: %+v", all)
	}
}

func readUntil(t *testing.T, ch <-chan []byte, needle []byte, timeout time.Duration) bool {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	var buf []byte
	for {
		select {
		case data, ok := <-ch:
			if !ok {
				return false
			}
			buf = append(buf, data...)
			if bytes.Contains(buf, needle) {
				return true
			}
		case <-timer.C:
			return false
		}
	}
}

// Renaming a session must carry its live panes with it. Leaving them under the
// old name splits the session in two: its attention roll-up, its layout and its
// delete would all look at the new name while the running shells still answered
// to the old one.
func TestRenameSessionMovesLivePanes(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	m := NewTerminalManager()
	pane, err := m.Open("old", 80, 24)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer m.KillAll()
	m.SetLayout("old", []byte("blob"), []string{pane})
	m.Mark("old", pane, protocol.AttentionWaiting, "needs you")

	m.RenameSession("old", "new")

	blob, alive := m.ListLayout("new")
	if string(blob) != "blob" {
		t.Errorf("layout blob = %q, want blob under the new name", blob)
	}
	if len(alive) != 1 || alive[0] != pane {
		t.Fatalf("alive under the new name = %v, want [%s]", alive, pane)
	}
	if _, oldAlive := m.ListLayout("old"); len(oldAlive) != 0 {
		t.Errorf("pane still listed under the old name: %v", oldAlive)
	}

	// The attention the pane reported belongs to the renamed session now.
	attn := m.SessionAttention()
	if attn["new"].Attention != protocol.AttentionWaiting {
		t.Errorf("attention did not move with the session: %+v", attn)
	}
	if _, ok := attn["old"]; ok {
		t.Error("attention still reported under the old session name")
	}

	// Attaching by the new name finds the existing pane rather than spawning a
	// second shell for it.
	before := len(m.sessions)
	if _, err := m.GetOrCreate("new", pane, 80, 24); err != nil {
		t.Fatalf("GetOrCreate after rename: %v", err)
	}
	if len(m.sessions) != before {
		t.Errorf("GetOrCreate spawned a duplicate pane: %d entries, want %d", len(m.sessions), before)
	}

	// And deleting the session reaches it.
	m.DropSession("new")
	if len(m.sessions) != 0 {
		t.Errorf("DropSession left %d panes behind after the rename", len(m.sessions))
	}
}

// A shell started before the rename still has the old RUNIC_SESSION in its
// environment, so `ru mark` arrives addressed to a session that no longer
// exists. The pane name is permanent, so the mark must still land.
func TestMarkAfterRenameResolvesByPaneName(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	m := NewTerminalManager()
	pane, err := m.Open("old", 80, 24)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer m.KillAll()

	m.RenameSession("old", "new")

	if n := m.Mark("old", pane, protocol.AttentionError, "crashed"); n != 1 {
		t.Fatalf("Mark with the stale session name touched %d panes, want 1", n)
	}
	if got := m.SessionAttention()["new"].Attention; got != protocol.AttentionError {
		t.Fatalf("attention = %v, want error on the renamed session", got)
	}
}

// The pane-name fallback must not guess: two sessions can each have a pane by
// the same name (the implicit "main"), and marking the wrong agent's pane is
// worse than marking none.
func TestMarkAmbiguousPaneNameIsRefused(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	m := NewTerminalManager()
	if _, err := m.GetOrCreate("a", "main", 80, 24); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	if _, err := m.GetOrCreate("b", "main", 80, 24); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	defer m.KillAll()

	if n := m.Mark("gone", "main", protocol.AttentionWaiting, ""); n != 0 {
		t.Fatalf("ambiguous pane name marked %d panes, want 0", n)
	}
}
