package tui

import (
	"testing"

	"github.com/hinshun/vt10x"

	"github.com/han/runic/internal/protocol"
)

// splitLayout builds a session layout of panes with the given daemon-side pane
// names, as a left-leaning tree of leaves. The shells carry a real vt and are
// marked done, so the resize that activateSession performs walks them without
// reaching for a PTY that this test never opened.
func splitLayout(panes ...string) *paneNode {
	var root *paneNode
	for _, p := range panes {
		leaf := newLeaf(&shellState{
			session: "agents",
			pane:    p,
			vt:      vt10x.New(vt10x.WithSize(20, 5)),
			done:    true,
		})
		if root == nil {
			root = leaf
			continue
		}
		root = &paneNode{dir: splitVert, children: []*paneNode{root, leaf}}
	}
	return root
}

func TestLeafWithPane(t *testing.T) {
	root := splitLayout("p1", "p2", "p3")
	for _, want := range []string{"p1", "p2", "p3"} {
		leaf := leafWithPane(root, want)
		if leaf == nil {
			t.Fatalf("pane %q not found in layout", want)
		}
		if leaf.shell.pane != want {
			t.Errorf("found pane %q, want %q", leaf.shell.pane, want)
		}
	}
	if leafWithPane(root, "nope") != nil {
		t.Error("an unknown pane should not match a leaf")
	}
	if leafWithPane(nil, "p1") != nil {
		t.Error("a nil layout should match nothing")
	}
	if leafWithPane(root, "") != nil {
		t.Error("an empty pane name should match nothing")
	}
}

// Enter on an agent row lands on that agent's own pane, not the session's first
// one. Without this, opening a blocked agent in a multi-pane session drops you
// on an unrelated shell.
func TestOpenAgentFocusesItsPane(t *testing.T) {
	m := model{viewMode: viewJobs, width: 100, height: 30}
	m.layouts = map[string]*paneNode{"agents": splitLayout("p1", "p2", "p3")}
	m.nodes = buildTree([]string{"work"},
		[]protocol.SessionInfo{{Name: "agents", Group: "work"}}, nil)
	m.agents = []protocol.TerminalInfo{
		{Session: "agents", Pane: "p1", Attention: protocol.AttentionWorking},
		{Session: "agents", Pane: "p3", Attention: protocol.AttentionWaiting, Note: "which schema?"},
	}
	m.jobs.tree.show = true
	m.jobs.tree.focus = true
	m.jobs.tree.tab = sideAgents
	m.refreshTreeRows()

	// p3 is waiting, so urgency sorting puts it first.
	cur, ok := m.jobs.tree.current()
	if !ok || cur.pane != "p3" {
		t.Fatalf("expected the waiting pane p3 under the cursor, got %+v", cur)
	}

	nm, _, _ := m.treeActivate()
	got := nm.(model)
	if got.activeSession != "agents" {
		t.Fatalf("activeSession = %q, want agents", got.activeSession)
	}
	if got.focusPane == nil || got.focusPane.shell == nil {
		t.Fatal("opening an agent should focus a pane")
	}
	if got.focusPane.shell.pane != "p3" {
		t.Errorf("focused pane %q, want p3 (the agent's own pane)", got.focusPane.shell.pane)
	}
}

// A session row has no pane of its own, so it still opens the session's first
// pane — the pane-targeting path must not change that.
func TestOpenSessionRowKeepsFirstPane(t *testing.T) {
	m := model{viewMode: viewJobs, width: 100, height: 30}
	m.layouts = map[string]*paneNode{"agents": splitLayout("p1", "p2")}
	m.nodes = buildTree([]string{"work"},
		[]protocol.SessionInfo{{Name: "agents", Group: "work"}}, nil)
	m.jobs.tree.show = true
	m.jobs.tree.focus = true
	m.jobs.tree.tab = sideSpaces
	m.refreshTreeRows()
	// rows: [group work, session agents] — put the cursor on the session.
	m.jobs.tree.cursor = 1
	cur, _ := m.jobs.tree.current()
	if cur.kind != treeSession {
		t.Fatalf("expected a session row, got kind %v", cur.kind)
	}

	nm, _, _ := m.treeActivate()
	got := nm.(model)
	if got.focusPane == nil || got.focusPane.shell.pane != "p1" {
		t.Errorf("a session row should open its first pane, got %+v", got.focusPane)
	}
}

// A pane that vanished between the poll and the keypress must not strand focus:
// the session still opens, on its first pane.
func TestOpenAgentWithStalePaneFallsBack(t *testing.T) {
	m := model{viewMode: viewJobs, width: 100, height: 30}
	m.layouts = map[string]*paneNode{"agents": splitLayout("p1")}
	m.nodes = buildTree([]string{"work"},
		[]protocol.SessionInfo{{Name: "agents", Group: "work"}}, nil)

	nm, _ := m.openSessionPane("agents", "gone")
	got := nm.(model)
	if got.activeSession != "agents" {
		t.Fatalf("the session should still open, got %q", got.activeSession)
	}
	if got.focusPane == nil || got.focusPane.shell.pane != "p1" {
		t.Errorf("a stale pane should fall back to the first leaf, got %+v", got.focusPane)
	}
}
