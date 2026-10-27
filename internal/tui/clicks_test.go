package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func leftClick(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
}

// Double-clicking a session in the sidebar opens it. A single click only
// selecting it is deliberate — a stray click must not launch into a session —
// but double-click is the gesture everyone tries first, and it did nothing.
func TestDoubleClickOpensSidebarSession(t *testing.T) {
	m := sideTabsModel()
	m.jobs.tree.tab = sideSpaces
	// Opening a session builds its panes, which newModel's maps back in the
	// real program; this fixture is jobs-view-only.
	m.layouts = map[string]*paneNode{}
	m.panes = map[int]*shellState{}
	m.refreshTreeRows()
	l := m.jobsLayout()

	row, y := -1, -1
	for i, r := range m.jobs.tree.rows {
		if r.kind == treeSession && r.session == "build" {
			row = i
			y = treeBodyTopLine + i - treeScrollOffset(m.jobs.tree.cursor, treeBodyHeight(l.paneH))
			break
		}
	}
	if row < 0 {
		t.Fatal("no session row in the tree")
	}

	// First click selects and nothing more.
	got, _ := m.handleMouse(leftClick(5, y))
	m = got.(model)
	if m.viewMode != viewJobs {
		t.Fatal("a single click must not open a session")
	}
	if r, ok := m.jobs.tree.current(); !ok || r.session != "build" {
		t.Fatalf("the click should have selected build, got %+v", r)
	}

	// Second click on the same row opens it.
	got, _ = m.handleMouse(leftClick(5, y))
	m = got.(model)
	if m.viewMode != viewSplit {
		t.Error("a double click should open the session")
	}
	if m.activeSession != "build" {
		t.Errorf("active session = %q, want build", m.activeSession)
	}
}

// Two clicks on *different* rows are two selections, not an activation — which
// is why the pair is tracked by row identity rather than by screen position.
func TestTwoClicksOnDifferentRowsDoNotOpen(t *testing.T) {
	m := sideTabsModel()
	m.jobs.tree.tab = sideSpaces
	m.refreshTreeRows()
	l := m.jobsLayout()
	bodyH := treeBodyHeight(l.paneH)
	if len(m.jobs.tree.rows) < 2 {
		t.Skip("need two rows")
	}
	off := treeScrollOffset(m.jobs.tree.cursor, bodyH)

	got, _ := m.handleMouse(leftClick(5, treeBodyTopLine+0-off))
	m = got.(model)
	got, _ = m.handleMouse(leftClick(5, treeBodyTopLine+1-off))
	m = got.(model)
	if m.viewMode != viewJobs {
		t.Error("clicks on two different rows must not open anything")
	}
}

// A third click starts a new pair rather than completing a second double, so
// a rapid triple-click opens exactly once.
func TestDoubleClickPairIsConsumed(t *testing.T) {
	var m model
	if (&m).doubleClick("a") {
		t.Fatal("the first click cannot be a double")
	}
	if !(&m).doubleClick("a") {
		t.Fatal("the second click on the same target is a double")
	}
	if (&m).doubleClick("a") {
		t.Error("the third click should start a fresh pair")
	}
}

// Only unambiguous key caps become buttons. A cap naming two keys has no single
// obvious action, and doing something arbitrary would be worse than nothing.
func TestHintPress(t *testing.T) {
	cases := []struct {
		cap          string
		key          string
		viaCtrlB, ok bool
	}{
		{cap: "⏎", key: "enter", ok: true},
		{cap: "tab", key: "tab", ok: true},
		{cap: "esc", key: "esc", ok: true},
		{cap: "space", key: " ", ok: true},
		{cap: "d", key: "d", ok: true},
		{cap: "?", key: "?", ok: true},
		{cap: "^B d", key: "d", viaCtrlB: true, ok: true},
		{cap: "^B s", key: "s", viaCtrlB: true, ok: true},
		// Ambiguous or chorded: not clickable.
		{cap: "j/k"},
		{cap: "gg/G"},
		{cap: "[/]"},
		{cap: "n/esc"},
		{cap: "^B |/-"},
		{cap: "^B n/p"},
		{cap: "^w h"},
		{cap: "any"},
	}
	for _, c := range cases {
		key, viaCtrlB, ok := hintPress(c.cap)
		if ok != c.ok || key != c.key || viaCtrlB != c.viaCtrlB {
			t.Errorf("hintPress(%q) = (%q, %v, %v), want (%q, %v, %v)",
				c.cap, key, viaCtrlB, ok, c.key, c.viaCtrlB, c.ok)
		}
	}
}

// The detach button: clicking `^B d` on the session view's status bar leaves the
// session, which previously required knowing the chord.
func TestClickDetachInSessionFooter(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	m := tabsSessionModel()
	m.width = 200 // wide enough that the status bar keeps its key caps
	m.refreshTreeRows()

	line, spans := m.footerSpans()
	if spans == nil {
		t.Fatal("the footer should report clickable spans at this width")
	}
	if !strings.Contains(stripAnsi(line), "detach") {
		t.Fatalf("the session footer should offer detach: %q", stripAnsi(line))
	}
	var target hintSpan
	for _, sp := range spans {
		if sp.hint.label == "detach" {
			target = sp
		}
	}
	if target.end == 0 {
		t.Fatal("no detach span")
	}
	// The span has to actually cover the rendered cap.
	if !strings.Contains(stripAnsi(line)[target.start:target.end], "d") {
		t.Errorf("the detach span does not cover its cap: %q", stripAnsi(line)[target.start:target.end])
	}

	got, _ := m.handleMouse(leftClick(target.start+1, m.height-1))
	if nm := got.(model); nm.viewMode != viewJobs {
		t.Error("clicking detach should return to the management view")
	}
}

// The management screen's hint bar is its toolbar: a click replays the key it
// advertises, through the same handler the keyboard uses.
func TestClickHintBarOnManagementScreen(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	m := tabsModel()
	m.width = 220 // wide enough that the whole hint bar is drawn
	m.refreshJobsRows()
	l := m.jobsLayout()
	_, spans := m.jobsFooterSpans(m.width)
	if len(spans) == 0 {
		t.Fatal("the hint bar should report spans")
	}
	var help hintSpan
	for _, sp := range spans {
		if sp.hint.key == "?" {
			help = sp
		}
	}
	if help.end == 0 {
		t.Fatalf("the ? hint should fit at %d columns", m.width)
	}
	got, _ := m.handleMouse(leftClick(help.start+1, l.paneH))
	if !got.(model).jobs.helping {
		t.Error("clicking the ? hint should open the help overlay")
	}
}

// Clicking a hint that names no single key does nothing at all, rather than
// guessing at one of them.
func TestClickAmbiguousHintIsInert(t *testing.T) {
	m := tabsModel()
	before := m.jobs.cursor
	_, _, handled := m.pressHint(keyHint{key: "j/k", label: "move"})
	if handled {
		t.Error("an ambiguous hint should not be actionable")
	}
	if m.jobs.cursor != before {
		t.Error("nothing should have moved")
	}
}

// With the terminal focused, a plain-key hint must not be typed into the shell.
func TestPlainHintDoesNotReachTheShell(t *testing.T) {
	m := tabsSessionModel()
	m.jobs.tree.focus = false
	if _, _, handled := m.pressHint(keyHint{key: "d", label: "delete"}); handled {
		t.Error("a plain key must not be dispatched while the shell has focus")
	}
}
