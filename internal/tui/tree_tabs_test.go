package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/han/runic/internal/protocol"
)

// sideTabsModel holds two groups of sessions and a pane in every attention
// state, so both sidebar views have something to show.
func sideTabsModel() model {
	now := time.Now()
	sessions := []protocol.SessionInfo{
		{Name: "default", Group: "default"},
		{Name: "build", Group: "work", Branch: "fix/login"},
	}
	jobs := []protocol.JobInfo{{ID: 1, Session: "build", Command: "make", State: protocol.StateRunning}}
	m := model{viewMode: viewJobs, width: 160, height: 40}
	m.nodes = buildTree([]string{"default", "work"}, sessions, jobs)
	m.jobs.allJobs = jobs
	// Deliberately out of urgency order, to prove the view sorts rather than
	// echoing the daemon's order.
	m.agents = []protocol.TerminalInfo{
		{Session: "default", Pane: "p1", Attention: protocol.AttentionWorking, Since: now.Add(-2 * time.Minute)},
		{Session: "build", Pane: "p1", Attention: protocol.AttentionWaiting, Note: "which schema?", Since: now.Add(-10 * time.Minute)},
		{Session: "build", Pane: "p2", Attention: protocol.AttentionError, Note: "tests failed", Since: now.Add(-time.Minute)},
		{Session: "default", Pane: "p2", Attention: protocol.AttentionIdle, Since: now.Add(-time.Hour)},
	}
	m.jobs.tree.show = true
	m.jobs.tree.focus = true
	m.refreshJobsRows()
	m.refreshTreeRows()
	return m
}

func agentKeys(rows []treeRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.session+"/"+r.pane)
	}
	return out
}

// The Agents view lists every live pane, most urgent first: errors, then panes
// waiting on you, then working, then quiet ones.
func TestAgentRowsSortByUrgency(t *testing.T) {
	m := sideTabsModel()
	want := "[build/p2 build/p1 default/p1 default/p2]"
	if got := fmt.Sprint(agentKeys(m.agentRows())); got != want {
		t.Errorf("agent order %s, want %s", got, want)
	}
}

// Within one state, the agent that has been there longest sorts first — a pane
// blocked for twenty minutes outranks one that just stopped.
func TestAgentRowsOldestFirstWithinState(t *testing.T) {
	now := time.Now()
	m := model{}
	m.agents = []protocol.TerminalInfo{
		{Session: "b", Pane: "p1", Attention: protocol.AttentionWaiting, Since: now.Add(-time.Minute)},
		{Session: "a", Pane: "p1", Attention: protocol.AttentionWaiting, Since: now.Add(-20 * time.Minute)},
	}
	if got := fmt.Sprint(agentKeys(m.agentRows())); got != "[a/p1 b/p1]" {
		t.Errorf("oldest wait should sort first, got %s", got)
	}
}

// A pane that has never reported a state is not an agent — it is just a shell.
// `ru mark` is how a process identifies itself, so an unmarked pane stays out
// of a list that claims to be agents.
func TestAgentRowsExcludeUnmarkedPanes(t *testing.T) {
	m := model{}
	m.agents = []protocol.TerminalInfo{
		{Session: "shell", Pane: "p1"}, // split off for a quick command
		{Session: "busy", Pane: "p1", Attention: protocol.AttentionWorking, Since: time.Now()},
	}
	if got := fmt.Sprint(agentKeys(m.agentRows())); got != "[busy/p1]" {
		t.Errorf("an unmarked pane should not be listed as an agent, got %s", got)
	}
	// The tab's badge has to agree with the rows it is counting.
	if n := m.sideTabCount(sideAgents); n != 1 {
		t.Errorf("Agents count %d, want 1", n)
	}
	// An idle agent still counts: it reported, so it is an agent with nothing
	// to do — different from a shell that never spoke.
	m.agents = append(m.agents, protocol.TerminalInfo{
		Session: "rested", Pane: "p1", Attention: protocol.AttentionIdle, Since: time.Now(),
	})
	if n := len(m.agentRows()); n != 2 {
		t.Errorf("an idle agent should still be listed, got %d rows", n)
	}
}

// Each sidebar tab builds its own row set: the fold tree, or the flat agent
// list. The tab's advertised count matches what it shows.
func TestSideTabRowSets(t *testing.T) {
	m := sideTabsModel()

	m.jobs.tree.tab = sideSpaces
	m.refreshTreeRows()
	// The Spaces tree now carries agents as children of their session — see
	// TestSpacesTreeNestsAgents for where they land.
	if n := m.sideTabCount(sideSpaces); n != len(m.nodes) {
		t.Errorf("Spaces count %d, want %d groups", n, len(m.nodes))
	}

	m.jobs.tree.tab = sideAgents
	m.refreshTreeRows()
	if len(m.jobs.tree.rows) != len(m.agents) {
		t.Fatalf("the Agents view should hold one row per pane, got %d of %d",
			len(m.jobs.tree.rows), len(m.agents))
	}
	for _, r := range m.jobs.tree.rows {
		if r.kind != treeAgent {
			t.Fatalf("the Agents view should hold only agent rows, got kind %v", r.kind)
		}
	}
	if n := m.sideTabCount(sideAgents); n != len(m.jobs.tree.rows) {
		t.Errorf("Agents count %d but %d rows", n, len(m.jobs.tree.rows))
	}
}

// An agent row carries the group of the session hosting it, so the forms and
// the detail pane have it without a second lookup.
func TestAgentRowCarriesGroup(t *testing.T) {
	m := sideTabsModel()
	for _, r := range m.agentRows() {
		want := "default"
		if r.session == "build" {
			want = "work"
		}
		if r.group != want {
			t.Errorf("%s/%s: group %q, want %q", r.session, r.pane, r.group, want)
		}
	}
}

// Tab and Shift+Tab roll the sidebar's views and wrap at both ends, while the
// tree has focus.
func TestSideTabsCycle(t *testing.T) {
	m := sideTabsModel()
	press := func(msg tea.KeyMsg) {
		nm, _ := m.handleJobsKey(msg)
		m = nm.(model)
	}
	if m.jobs.tree.tab != sideSpaces {
		t.Fatalf("the sidebar should open on Spaces, got %v", m.jobs.tree.tab)
	}
	press(tea.KeyMsg{Type: tea.KeyTab})
	if m.jobs.tree.tab != sideAgents {
		t.Fatalf("tab should roll to Agents, got %v", m.jobs.tree.tab)
	}
	// Wraps forward back to the first tab.
	press(tea.KeyMsg{Type: tea.KeyTab})
	if m.jobs.tree.tab != sideSpaces {
		t.Fatalf("tab should wrap to Spaces, got %v", m.jobs.tree.tab)
	}
	// And backward off the first tab.
	press(tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.jobs.tree.tab != sideAgents {
		t.Fatalf("shift+tab should wrap to Agents, got %v", m.jobs.tree.tab)
	}
	if !m.jobs.tree.focus {
		t.Error("rolling the sidebar's views should not move focus")
	}
}

// With the table focused, Tab still rolls the job views — the key belongs to
// whichever pane has focus.
func TestSideTabsLeaveJobViewsToTheTable(t *testing.T) {
	m := sideTabsModel()
	m.jobs.tree.focus = false
	nm, _ := m.handleJobsKey(tea.KeyMsg{Type: tea.KeyTab})
	m = nm.(model)
	if m.jobs.tab != tabRunning {
		t.Errorf("tab with the table focused should roll the job view, got %v", m.jobs.tab)
	}
	if m.jobs.tree.tab != sideSpaces {
		t.Errorf("tab with the table focused should leave the sidebar alone, got %v", m.jobs.tree.tab)
	}
}

// The arrow keys roll the job views from the table only; in the sidebar they
// stay fold keys and must not reach the tabs.
func TestSideTabsIgnoreArrows(t *testing.T) {
	m := sideTabsModel()
	for _, key := range []tea.KeyType{tea.KeyLeft, tea.KeyRight} {
		nm, _ := m.handleJobsKey(tea.KeyMsg{Type: key})
		got := nm.(model)
		if got.jobs.tab != tabAll || got.jobs.tree.tab != sideSpaces {
			t.Errorf("%v in the sidebar should roll neither set of tabs (job=%v side=%v)",
				key, got.jobs.tab, got.jobs.tree.tab)
		}
	}
}

// Switching views parks the cursor at the top: the two tabs hold unrelated
// rows, so a carried-over index would land somewhere arbitrary.
func TestSideTabSwitchResetsCursor(t *testing.T) {
	m := sideTabsModel()
	m.jobs.tree.cursor = 2
	(&m).switchSideTab(sideAgents)
	if m.jobs.tree.cursor != 0 {
		t.Errorf("cursor %d after switching views, want 0", m.jobs.tree.cursor)
	}
}

// The sidebar header is a tab strip: the active tab is a chip carrying its
// count, and the Agents tab shows its "needs you" count from the Spaces view.
func TestSideTabStripRender(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	m := sideTabsModel()

	strip, spans := m.renderSideTabStrip(treePaneWidth)
	plain := stripAnsi(strip)
	if !strings.Contains(plain, " Spaces 2 ") {
		t.Errorf("active tab should carry its count: %q", plain)
	}
	// Two panes need you: build/p1 is waiting and build/p2 errored.
	if !strings.Contains(plain, "Agents 2!") {
		t.Errorf("inactive Agents tab should carry the needs-you count: %q", plain)
	}
	if len(spans) != int(numSideTabs) {
		t.Fatalf("got %d spans, want one per tab", len(spans))
	}
	if lipgloss.Width(strip) > treePaneWidth {
		t.Errorf("strip is %d columns, wider than the sidebar's %d", lipgloss.Width(strip), treePaneWidth)
	}
}

// Clicking a sidebar tab switches to it and hands the sidebar focus.
func TestSideTabClick(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	m := sideTabsModel()
	m.jobs.tree.focus = false

	l := m.jobsLayout()
	_, spans := m.renderSideTabStrip(l.treeW - 2)
	target := spans[sideAgents]
	nm, _ := m.handleMouse(tea.MouseMsg{
		X: 1 + target.start, Y: mgmtTabStripLine,
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
	})
	got := nm.(model)
	if got.jobs.tree.tab != sideAgents {
		t.Errorf("clicking the Agents tab should switch to it, got %v", got.jobs.tree.tab)
	}
	if !got.jobs.tree.focus {
		t.Error("clicking the sidebar should hand it focus")
	}
}

// Clicking a sidebar row selects it and focuses the pane, but does not open it:
// a stray click must not drop you into a session.
func TestSideRowClickSelectsWithoutOpening(t *testing.T) {
	m := sideTabsModel()
	m.jobs.tree.focus = false

	nm, cmd := m.handleMouse(tea.MouseMsg{
		X: 2, Y: treeBodyTopLine + 1,
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
	})
	got := nm.(model)
	if got.jobs.tree.cursor != 1 {
		t.Errorf("cursor %d, want the clicked row (1)", got.jobs.tree.cursor)
	}
	if !got.jobs.tree.focus {
		t.Error("clicking a sidebar row should hand the pane focus")
	}
	if cmd != nil {
		t.Error("clicking a sidebar row should not open anything")
	}
	if got.activeSession != "" {
		t.Errorf("clicking a sidebar row should not activate a session, got %q", got.activeSession)
	}
}

// The Agents view is flat, so its hint bar drops the fold and group keys rather
// than advertising keys that do nothing there.
func TestAgentViewHints(t *testing.T) {
	m := sideTabsModel()
	m.jobs.tree.tab = sideAgents
	m.refreshTreeRows()

	var keys []string
	for _, h := range m.jobsHints() {
		keys = append(keys, h.key)
	}
	joined := strings.Join(keys, " ")
	for _, gone := range []string{"l/h", "n", "d"} {
		if strings.Contains(" "+joined+" ", " "+gone+" ") {
			t.Errorf("the Agents view should not hint %q: %s", gone, joined)
		}
	}
	if !strings.Contains(joined, "tab") {
		t.Errorf("the Agents view should still hint tab: %s", joined)
	}
}

// An empty Agents tab says how it gets filled instead of rendering blank.
func TestAgentEmptyState(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	m := sideTabsModel()
	m.agents = nil
	m.jobs.tree.tab = sideAgents
	m.refreshTreeRows()

	body := stripAnsi(strings.Join(m.treePaneLines(10, treePaneWidth), "\n"))
	headline, hint := agentEmptyMessage()
	if !strings.Contains(body, headline) {
		t.Errorf("empty Agents tab should explain itself, got %q", body)
	}
	if !strings.Contains(body, "ru mark") {
		t.Errorf("empty Agents tab should hint ru mark, got %q (hint: %q)", body, hint)
	}
}

// An agent row gets its own detail panel — the session panel's worktree and job
// fields are meaningless for a single pane.
func TestAgentDetailPanel(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	m := sideTabsModel()
	m.jobs.tree.tab = sideAgents
	m.refreshTreeRows()
	m.jobs.tree.cursor = 0 // build/p2, the errored pane

	lines, title, ok := m.sessionDetailLines(60)
	if !ok {
		t.Fatal("the sidebar should drive the detail panel while focused")
	}
	if strings.TrimSpace(title) != "agent" {
		t.Errorf("an agent row should head its panel %q, not %q", "agent", strings.TrimSpace(title))
	}
	body := stripAnsi(strings.Join(lines, "\n"))
	for _, want := range []string{"build · p2", "tests failed", "error"} {
		if !strings.Contains(body, want) {
			t.Errorf("agent detail should mention %q, got %q", want, body)
		}
	}
	if strings.Contains(body, "Isolation") {
		t.Errorf("agent detail should not claim the session's isolation field: %q", body)
	}
}

// Two agents in one session are two rows, distinguished by pane; a session with
// one agent needs no pane suffix.
func TestAgentLabelDisambiguatesPanes(t *testing.T) {
	m := sideTabsModel()
	m.agents = []protocol.TerminalInfo{
		{Session: "solo", Pane: "p1", Attention: protocol.AttentionWorking},
		{Session: "pair", Pane: "p1", Attention: protocol.AttentionWorking},
		{Session: "pair", Pane: "p2", Attention: protocol.AttentionWorking},
	}
	for _, r := range m.agentRows() {
		got := m.agentLabel(r)
		want := r.session
		if r.session == "pair" {
			want = r.session + " · " + r.pane
		}
		if got != want {
			t.Errorf("agentLabel = %q, want %q", got, want)
		}
	}
}
