package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/hinshun/vt10x"
	"github.com/muesli/termenv"
)

// zoomModel is a session with two panes side by side. The tree is built
// directly: splitFocused opens a PTY on the daemon, which these tests have not
// got.
func zoomModel(t *testing.T) model {
	t.Helper()
	m := tabsSessionModel()
	m.width, m.height = 120, 30
	left := firstLeaf(m.layouts["agent-2"])
	right := newLeaf(&shellState{session: "agent-2", pane: "p2", vt: vt10x.New(vt10x.WithSize(20, 5)), done: true})
	m.layouts["agent-2"] = &paneNode{dir: splitVert, children: []*paneNode{left, right}}
	m.focusPane = left
	m.shell = left.shell
	if m.paneCount() != 2 {
		t.Fatalf("expected two panes, got %d", m.paneCount())
	}
	return m
}

// Zoom gives the focused pane the whole region. The point is the PTY, not the
// drawing: a shell still sized for its old rectangle would redraw a full-screen
// agent into a corner.
func TestZoomResizesThePaneItShows(t *testing.T) {
	m := zoomModel(t)
	rw, rh := m.termRegionSize()
	iw, ih := rw-2, rh-2

	split, _ := m.termLeaves(iw, ih)
	if len(split) != 2 {
		t.Fatalf("expected two leaves unzoomed, got %d", len(split))
	}
	for _, lr := range split {
		if lr.rect.w >= iw {
			t.Error("neither pane should own the full width when split")
		}
	}

	m.paneZoom = true
	zoomed, seps := m.termLeaves(iw, ih)
	if len(zoomed) != 1 {
		t.Fatalf("zoomed should lay out one pane, got %d", len(zoomed))
	}
	if zoomed[0].node != m.focusPane {
		t.Error("zoom should show the focused pane")
	}
	if zoomed[0].rect.w != iw || zoomed[0].rect.h != ih {
		t.Errorf("zoomed rect = %dx%d, want the whole region %dx%d",
			zoomed[0].rect.w, zoomed[0].rect.h, iw, ih)
	}
	if len(seps) != 0 {
		t.Error("a single pane has no separators to draw")
	}
}

// Ctrl+B z toggles, and says so on the status bar — zoom hides the other panes,
// so the fact that they exist has to stay on screen.
func TestZoomChordAndIndicator(t *testing.T) {
	lipgloss.SetColorProfile(termenv.Ascii)
	m := zoomModel(t)

	press := func(m model) model {
		m.ctrlBPressed = true
		nm, handled, _ := m.handleCtrlB(keyRune('z'))
		if !handled {
			t.Fatal("Ctrl+B z was not handled")
		}
		return nm
	}
	m = press(m)
	if !m.paneZoom {
		t.Fatal("Ctrl+B z should zoom")
	}
	if got := stripAnsi(mustLine(t, m)); !strings.Contains(got, "ZOOM") {
		t.Errorf("the status bar should say ZOOM: %q", got)
	}
	m = press(m)
	if m.paneZoom {
		t.Error("Ctrl+B z again should restore the layout")
	}
	if got := stripAnsi(mustLine(t, m)); strings.Contains(got, "ZOOM") {
		t.Errorf("the indicator should go with the zoom: %q", got)
	}
}

// With one pane there is nothing to zoom away from, and silently doing nothing
// would read as a broken key.
func TestZoomRefusedWithOnePane(t *testing.T) {
	m := tabsSessionModel()
	m.ctrlBPressed = true
	nm, handled, _ := m.handleCtrlB(keyRune('z'))
	if !handled {
		t.Fatal("the chord should still be consumed")
	}
	if nm.paneZoom {
		t.Error("a single pane must not zoom")
	}
	if nm.status == "" {
		t.Error("refusing should say why")
	}
}

// Zoom belongs to a layout, so it cannot outlive the layout changing under it.
func TestZoomReleasedWhenLayoutChanges(t *testing.T) {
	cases := []struct {
		name string
		act  func(model) model
	}{
		{"splitting", func(m model) model { nm, _ := m.splitFocused(splitHoriz); return nm }},
		{"closing the pane", func(m model) model { nm, _ := m.closeFocused(); return nm }},
		{"switching session", func(m model) model { nm, _ := m.activateSession("agent-1"); return nm }},
	}
	for _, c := range cases {
		m := zoomModel(t)
		m.paneZoom = true
		if got := c.act(m); got.paneZoom {
			t.Errorf("%s should release zoom", c.name)
		}
	}
}

// The command line reaches it too, for anyone who prefers words to chords.
func TestZoomCommand(t *testing.T) {
	m := zoomModel(t)
	got, _ := m.executeCommand("zoom")
	if !got.(model).paneZoom {
		t.Error(":zoom should zoom")
	}
	got, _ = m.executeCommand("z")
	if !got.(model).paneZoom {
		t.Error(":z should zoom")
	}
}
