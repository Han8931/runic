package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/hinshun/vt10x"
	"github.com/muesli/termenv"

	"github.com/han/runic/internal/protocol"
)

// tabsSessionModel opens four sessions with a pane each and a spread of
// attention states, with agent-2 active.
func tabsSessionModel() model {
	now := time.Now()
	sessions := []protocol.SessionInfo{
		{Name: "default", Group: "default"},
		{Name: "agent-1", Group: "work",
			Attention: protocol.AttentionWaiting, AttentionNote: "which schema?",
			AttentionSince: now.Add(-14 * time.Minute)},
		{Name: "agent-2", Group: "work",
			Attention: protocol.AttentionWorking, AttentionSince: now.Add(-3 * time.Minute)},
		{Name: "broken", Group: "work",
			Attention: protocol.AttentionError, AttentionNote: "tests failed",
			AttentionSince: now.Add(-time.Minute), AttentionSeen: true},
	}
	m := model{viewMode: viewSplit, width: 96, height: 16, focus: paneTerm}
	m.nodes = buildTree([]string{"default", "work"}, sessions, nil)
	m.layouts = map[string]*paneNode{}
	for _, s := range sessions {
		m.layouts[s.Name] = newLeaf(&shellState{
			session: s.Name, pane: "p1",
			vt: vt10x.New(vt10x.WithSize(60, 10)), done: true,
		})
	}
	m.activeSession = "agent-2"
	m.focusPane = firstLeaf(m.layouts["agent-2"])
	m.shell = m.focusPane.shell
	return m
}

// openSessionTabs reports only the sessions holding a live layout — the
// pre-catalog fallback for the tab list. Order is by name, matching
// sessionNames, so a digit keeps pointing at the same place.
func TestOpenSessionTabs(t *testing.T) {
	m := tabsSessionModel()
	got := strings.Join(m.openSessionTabs(), ",")
	if got != "agent-1,agent-2,broken,default" {
		t.Errorf("tabs = %q, want them sorted by name", got)
	}

	// A session with no layout is not open and must not appear.
	m.layouts["never-opened"] = nil
	if strings.Contains(strings.Join(m.openSessionTabs(), ","), "never-opened") {
		t.Error("a session without a layout should not be a tab")
	}
}

func TestCycleSessionTab(t *testing.T) {
	m := tabsSessionModel() // active: agent-2, index 1 of 4
	nm, _ := m.cycleSessionTab(1)
	if nm.activeSession != "broken" {
		t.Errorf("next = %q, want broken", nm.activeSession)
	}
	nm, _ = m.cycleSessionTab(-1)
	if nm.activeSession != "agent-1" {
		t.Errorf("prev = %q, want agent-1", nm.activeSession)
	}
	// Wrapping at both ends.
	m.activeSession = "default" // last
	nm, _ = m.cycleSessionTab(1)
	if nm.activeSession != "agent-1" {
		t.Errorf("wrap forward = %q, want agent-1", nm.activeSession)
	}
	m.activeSession = "agent-1" // first
	nm, _ = m.cycleSessionTab(-1)
	if nm.activeSession != "default" {
		t.Errorf("wrap backward = %q, want default", nm.activeSession)
	}
}

// One open session has nowhere to cycle to, and must not be re-activated (which
// would pointlessly reset focus to its first pane).
func TestCycleSessionTabSingleIsNoop(t *testing.T) {
	m := tabsSessionModel()
	m.layouts = map[string]*paneNode{"agent-2": m.layouts["agent-2"]}
	m.nodes = nil // the catalog, as well as the open layouts, has one session
	nm, cmd := m.cycleSessionTab(1)
	if nm.activeSession != "agent-2" {
		t.Errorf("active = %q, want it unchanged", nm.activeSession)
	}
	if cmd != nil {
		t.Error("cycling a single tab should do nothing at all")
	}
}

func TestSelectSessionTab(t *testing.T) {
	m := tabsSessionModel()
	nm, _ := m.selectSessionTab(3) // 1-based: broken
	if nm.activeSession != "broken" {
		t.Errorf("tab 3 = %q, want broken", nm.activeSession)
	}
	// Out of range, in both directions, leaves the active session alone.
	for _, n := range []int{0, -1, 5, 99} {
		nm, cmd := m.selectSessionTab(n)
		if nm.activeSession != "agent-2" || cmd != nil {
			t.Errorf("tab %d should be a no-op, got %q", n, nm.activeSession)
		}
	}
	// Selecting the active tab changes nothing.
	if _, cmd := m.selectSessionTab(2); cmd != nil {
		t.Error("selecting the active tab should be a no-op")
	}
}

// The strip names every session and marks the active one.
func TestSessionStripNamesEverySession(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	m := tabsSessionModel()

	strip, spans := m.sessionStrip(96, m.sessionNames(), m.activeSession)
	plain := stripAnsi(strip)
	for _, name := range []string{"agent-1", "agent-2", "broken", "default"} {
		if !strings.Contains(plain, name) {
			t.Errorf("strip missing %q: %q", name, plain)
		}
	}
	if len(spans) != 4 {
		t.Errorf("got %d spans, want one per session", len(spans))
	}
	if w := lipgloss.Width(strip); w != 96 {
		t.Errorf("strip is %d columns, want 96", w)
	}
}

// A waiting agent wears its glyph on an inactive tab — that is the point of the
// strip: noticing it without leaving the session you are in.
func TestSessionStripShowsForeignAttention(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	m := tabsSessionModel()
	strip, _ := m.sessionStrip(96, m.sessionNames(), m.activeSession)
	plain := stripAnsi(strip)

	waiting, _, _ := attentionMark(protocol.AttentionWaiting)
	if !strings.Contains(plain, waiting+" agent-1") {
		t.Errorf("agent-1 is waiting and should carry %q: %q", waiting, plain)
	}
	// An acknowledged error is not news, so it gets no glyph.
	errGlyph, _, _ := attentionMark(protocol.AttentionError)
	if strings.Contains(plain, errGlyph+" broken") {
		t.Errorf("an acknowledged error should not wear an alert glyph: %q", plain)
	}
}

func TestSessionAttentionForTabs(t *testing.T) {
	m := tabsSessionModel()
	cases := map[string]protocol.Attention{
		"agent-1": protocol.AttentionWaiting, // unresolved
		"agent-2": protocol.AttentionWorking, // informational
		"broken":  protocol.AttentionNone,    // error, acknowledged
		"default": protocol.AttentionNone,    // nothing reported
		"absent":  protocol.AttentionNone,    // not a session at all
	}
	for name, want := range cases {
		if got := m.sessionAttention(name); got != want {
			t.Errorf("sessionAttention(%q) = %v, want %v", name, got, want)
		}
	}
}

// However narrow the region, the strip occupies exactly its width — a short
// line would leave the box's top border ragged.
func TestSessionStripFitsAnyWidth(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	m := tabsSessionModel()
	for _, w := range []int{0, 1, 4, 7, 20, 30, 96} {
		strip, _ := m.sessionStrip(w, m.sessionNames(), m.activeSession)
		if got := lipgloss.Width(strip); got != w {
			t.Errorf("width %d: strip rendered %d columns", w, got)
		}
	}
	// No sessions at all is a blank strip, not a panic.
	m.nodes, m.layouts = nil, nil
	if strip, spans := m.sessionStrip(96, m.sessionNames(), ""); lipgloss.Width(strip) != 96 || spans != nil {
		t.Error("an empty catalog should render a blank full-width strip")
	}
}

// The chords are wired: Ctrl+B n/p roll the tabs and Ctrl+B <digit> jumps.
// Driving handleCtrlB rather than the helpers proves the keys actually reach
// them and are not shadowed by an existing chord.
func TestCtrlBSessionTabChords(t *testing.T) {
	press := func(m model, key string) model {
		m.ctrlBPressed = true
		nm, handled, _ := m.handleCtrlB(keyRune(rune(key[0])))
		if !handled {
			t.Fatalf("Ctrl+B %s was not handled", key)
		}
		return nm
	}
	m := tabsSessionModel() // active agent-2, tabs: agent-1 agent-2 broken default
	if got := press(m, "n").activeSession; got != "broken" {
		t.Errorf("Ctrl+B n = %q, want broken", got)
	}
	if got := press(m, "p").activeSession; got != "agent-1" {
		t.Errorf("Ctrl+B p = %q, want agent-1", got)
	}
	if got := press(m, "4").activeSession; got != "default" {
		t.Errorf("Ctrl+B 4 = %q, want default", got)
	}
	// A digit past the end is a no-op, not a crash.
	if got := press(m, "9").activeSession; got != "agent-2" {
		t.Errorf("Ctrl+B 9 = %q, want the active session unchanged", got)
	}
}

// Switching tabs must land focus inside the new session, not leave it pointing
// at a pane from the old one.
func TestSessionTabSwitchMovesFocus(t *testing.T) {
	m := tabsSessionModel()
	nm, _ := m.cycleSessionTab(1) // -> broken
	if nm.focusPane == nil || nm.shell == nil {
		t.Fatal("switching tabs should leave a focused pane")
	}
	if nm.focusPane.shell.session != "broken" {
		t.Errorf("focus stayed in %q after switching to broken", nm.focusPane.shell.session)
	}
	if nm.shell != nm.focusPane.shell {
		t.Error("m.shell should track the newly focused pane")
	}
}

// The rendered session view carries the tabs on its first line.
func TestSessionViewHasTabs(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	m := tabsSessionModel()
	first := stripAnsi(strings.SplitN(m.View(), "\n", 2)[0])
	if !strings.Contains(first, "agent-1") || !strings.Contains(first, "default") {
		t.Errorf("the session view's top line should carry the tabs: %q", first)
	}
}
