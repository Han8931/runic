package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/han/runic/internal/protocol"
)

// agentsViewModel is a screen with four agents in distinct states.
func agentsViewModel() model {
	lipgloss.SetColorProfile(termenv.Ascii)
	now := time.Now()
	sessions := []protocol.SessionInfo{
		{Name: "review-api", Group: "work", Agent: "claude", Branch: "feature/api"},
		{Name: "fix-tests", Group: "work", Agent: "claude", Branch: "feature/fix"},
		{Name: "lint", Group: "work", Agent: "codex", Branch: "chore/lint"},
		{Name: "default", Group: "default"},
	}
	m := model{viewMode: viewJobs, width: 110, height: 30}
	m.nodes = buildTree([]string{"default", "work"}, sessions, nil)
	m.agents = []protocol.TerminalInfo{
		{Session: "review-api", Pane: "p1", Attention: protocol.AttentionWaiting,
			Note:  "Should this endpoint require authentication?",
			Since: now.Add(-8 * time.Minute), LastOutput: now.Add(-8 * time.Minute)},
		{Session: "fix-tests", Pane: "p1", Attention: protocol.AttentionWorking,
			Since: now.Add(-2 * time.Minute), LastOutput: now.Add(-3 * time.Second)},
		{Session: "lint", Pane: "p1", Attention: protocol.AttentionError,
			Note:  "exit 1: 3 lint failures in internal/tui",
			Since: now.Add(-50 * time.Second), LastOutput: now.Add(-50 * time.Second)},
		{Session: "default", Pane: "p1", Attention: protocol.AttentionWorking,
			Since: now.Add(-30 * time.Minute), LastOutput: now.Add(-22 * time.Minute)},
	}
	m.jobs.main = mainAgents
	m.jobs.tree.show = true
	m.refreshJobsRows()
	m.refreshTreeRows()
	return m
}

func agentViewText(m model, inner, bodyH int) string {
	return stripAnsi(strings.Join(m.renderAgentView(inner, bodyH), "\n"))
}

// The reason the view exists: the question an agent is blocked on is readable
// in full. In the 26-column sidebar a name, a state and an age already fill the
// row, so the request had nowhere to go.
func TestAgentViewShowsTheWholeRequest(t *testing.T) {
	m := agentsViewModel()
	got := agentViewText(m, 80, 12)
	for _, want := range []string{
		"Should this endpoint require authentication?",
		"exit 1: 3 lint failures in internal/tui",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the request should be readable in full:\nwant %q in\n%s", want, got)
		}
	}
}

// Urgency first, same order as the sidebar's — one answer to "who needs me",
// wherever it is asked.
func TestAgentViewOrdersByUrgency(t *testing.T) {
	m := agentsViewModel()
	rows := m.agentViewRows()
	if len(rows) != 4 {
		t.Fatalf("got %d rows, want 4", len(rows))
	}
	if rows[0].session != "lint" {
		t.Errorf("first row = %q, want the unresolved error", rows[0].session)
	}
	if rows[1].session != "review-api" {
		t.Errorf("second row = %q, want the waiting agent", rows[1].session)
	}
}

// Two lines by default, one on request. Density is for triaging a screenful of
// agents without the second lines doubling the scrolling.
func TestAgentViewDensity(t *testing.T) {
	m := agentsViewModel()
	if m.agentRowHeight() != 2 {
		t.Error("two lines should be the default")
	}
	comfortable := agentViewText(m, 80, 12)

	got, _, handled := m.handleAgentViewKey(keyRune('c'))
	if !handled {
		t.Fatal("c should toggle density")
	}
	m = got.(model)
	if m.agentRowHeight() != 1 {
		t.Error("c should compact the rows")
	}
	compact := agentViewText(m, 80, 12)

	// Compact fits more agents and drops the request lines.
	if strings.Contains(compact, "Should this endpoint") {
		t.Error("compact rows should not carry the request line")
	}
	if strings.Count(comfortable, "review-api") == 0 || strings.Count(compact, "review-api") == 0 {
		t.Error("both densities should list every agent")
	}
	// And back again.
	got, _, _ = m.handleAgentViewKey(keyRune('c'))
	if got.(model).agentRowHeight() != 2 {
		t.Error("c again should restore two-line rows")
	}
}

// Rendering fills exactly the pane it was given, at any size: an overflow
// corrupts the frame around it.
func TestAgentViewFitsItsPane(t *testing.T) {
	m := agentsViewModel()
	for _, inner := range []int{40, 60, 80, 120} {
		for _, bodyH := range []int{1, 3, 8, 20} {
			lines := m.renderAgentView(inner, bodyH)
			if len(lines) != bodyH {
				t.Errorf("%dx%d: rendered %d lines", inner, bodyH, len(lines))
			}
			for i, l := range lines {
				if w := lipgloss.Width(l); w != inner {
					t.Errorf("%dx%d: line %d is %d columns: %q", inner, bodyH, i, w, stripAnsi(l))
				}
			}
		}
	}
}

// The optional columns are dropped rather than squeezed, like the job table's.
func TestAgentViewDropsColumnsWhenNarrow(t *testing.T) {
	m := agentsViewModel()
	wide := agentViewText(m, 110, 10)
	narrow := agentViewText(m, 44, 10)

	if !strings.Contains(wide, "claude") {
		t.Error("the kind should show when there is room")
	}
	if strings.Contains(narrow, "feature/api") {
		t.Error("the branch should be dropped rather than squeezed at 44 columns")
	}
	// What the row *is* always survives.
	for _, width := range []int{44, 60, 110} {
		if got := agentViewText(m, width, 10); !strings.Contains(got, "review-api") {
			t.Errorf("%d: the agent's name must always survive:\n%s", width, got)
		}
	}
}

// Enter attaches to the agent under the cursor, and `!` acknowledges it.
func TestAgentViewActions(t *testing.T) {
	m := agentsViewModel()
	m.layouts = map[string]*paneNode{}
	m.panes = map[int]*shellState{}

	// Cursor starts on the errored agent.
	r, ok := m.agentCursorRow()
	if !ok || r.session != "lint" {
		t.Fatalf("cursor is on %+v", r)
	}
	_, cmd, handled := m.handleAgentViewKey(keyRune('!'))
	if !handled || cmd == nil {
		t.Error("! should acknowledge an unresolved agent")
	}

	// Moving down lands on the waiting one.
	got, _, _ := m.handleAgentViewKey(keyRune('j'))
	m = got.(model)
	if r, _ := m.agentCursorRow(); r.session != "review-api" {
		t.Fatalf("j moved to %q", r.session)
	}
	got, _, handled = m.handleAgentViewKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled {
		t.Fatal("enter should be handled")
	}
	if nm := got.(model); nm.viewMode != viewSplit || nm.activeSession != "review-api" {
		t.Errorf("enter should attach to the agent's session, got %q in view %v",
			nm.activeSession, nm.viewMode)
	}
}

// A calm agent has nothing to acknowledge, so the key is inert rather than
// firing a request the daemon would refuse.
func TestAgentViewAckOnlyWhenUnresolved(t *testing.T) {
	m := agentsViewModel()
	rows := m.agentViewRows()
	for i, r := range rows {
		if protocol.Unresolved(r.attention, r.seen) {
			continue
		}
		m.jobs.agentCursor = i
		if _, cmd, _ := m.handleAgentViewKey(keyRune('!')); cmd != nil {
			t.Errorf("! on %v should do nothing", r.attention)
		}
	}
}

// Job actions are not claimed here: their rows are panes, so kill, rerun and
// urgent have nothing to act on.
func TestAgentViewLeavesJobActionsAlone(t *testing.T) {
	m := agentsViewModel()
	for _, key := range []rune{'x', 'u', 'r', 'd'} {
		if _, _, handled := m.handleAgentViewKey(keyRune(key)); handled {
			t.Errorf("%q should fall through to the shared bindings", key)
		}
	}
}

// `a` reaches the view in one keystroke. It used to be a second way to say `n`,
// while getting to the agent list meant focusing the sidebar and rolling tabs.
func TestAgentViewReachedWithA(t *testing.T) {
	m := agentsViewModel()
	m.jobs.main = mainJobs
	got, _ := m.handleJobsKey(keyRune('a'))
	if got.(model).jobs.main != mainAgents {
		t.Error("a should switch the main pane to Agents")
	}
}

// The primary tabs carry the counts, and a blocked agent is visible from the
// Jobs view — otherwise the tab only helps once you already suspect something.
func TestMainTabStripAdvertisesBlockedAgents(t *testing.T) {
	m := agentsViewModel()
	m.jobs.main = mainJobs
	strip, jobSpans, mainSpans := m.renderMainTabStrip(90)
	plain := stripAnsi(strip)
	if !strings.Contains(plain, "Agents") || !strings.Contains(plain, "2!") {
		t.Errorf("the Agents tab should carry its needs-you count: %q", plain)
	}
	if len(mainSpans) != int(numMainViews) {
		t.Errorf("got %d primary spans, want %d", len(mainSpans), numMainViews)
	}
	// Jobs is showing, so its state filters are offered too.
	if len(jobSpans) != int(numJobsTabs) {
		t.Errorf("got %d job spans, want %d", len(jobSpans), numJobsTabs)
	}

	// Switched to Agents, the state filters belong to a view that is not
	// showing, so they go.
	m.jobs.main = mainAgents
	_, jobSpans, _ = m.renderMainTabStrip(90)
	if len(jobSpans) != 0 {
		t.Error("the job filters should not be offered while Agents is showing")
	}
}

// Clicking a primary tab switches view; clicking a state filter still filters.
func TestMainTabClicks(t *testing.T) {
	m := agentsViewModel()
	m.jobs.main = mainJobs
	l := m.jobsLayout()
	_, jobSpans, mainSpans := m.renderMainTabStrip(l.listW - 2)

	var agentsTab mainTabSpan
	for _, sp := range mainSpans {
		if sp.view == mainAgents {
			agentsTab = sp
		}
	}
	got, _ := m.handleMouse(leftClick(l.listX+1+agentsTab.start, mgmtTabStripLine))
	if got.(model).jobs.main != mainAgents {
		t.Error("clicking the Agents tab should switch to it")
	}

	target := jobSpans[tabFailed]
	got, _ = m.handleMouse(leftClick(l.listX+1+target.start, mgmtTabStripLine))
	if nm := got.(model); nm.jobs.tab != tabFailed || nm.jobs.main != mainJobs {
		t.Error("clicking a state filter should filter Jobs, not switch views")
	}
}

func TestReturnToJobs(t *testing.T) {
	for _, key := range []tea.KeyType{tea.KeyTab, tea.KeyShiftTab} {
		m := agentsViewModel()
		got, _ := m.handleJobsKey(tea.KeyMsg{Type: key})
		if got.(model).jobs.main != mainJobs {
			t.Fatalf("%v did not return to Jobs", key)
		}
	}
	for _, width := range []int{60, 110, 160} {
		m := agentsViewModel()
		m.width = width
		m.jobs.tree.focus = true
		l := m.jobsLayout()
		_, _, spans := m.renderMainTabStrip(l.listW - 2)
		for _, sp := range spans {
			if sp.view != mainJobs {
				continue
			}
			got, _ := m.handleMouse(leftClick(l.listX+1+sp.start, mgmtTabStripLine))
			nm := got.(model)
			if nm.jobs.main != mainJobs || nm.jobs.tree.focus {
				t.Fatalf("width %d: clicking Jobs did not switch and focus the main pane", width)
			}
		}
	}
}

// Locate the visible labels in the complete frame rather than deriving click
// coordinates from the same spans used by the mouse handler.
func TestRenderedMainTabsAcceptClicks(t *testing.T) {
	for _, width := range []int{40, 60, 110, 160} {
		for _, showTree := range []bool{false, true} {
			m := agentsViewModel()
			m.width = width
			m.jobs.tree.show = showTree
			for _, target := range []mainView{mainJobs, mainAgents, mainJobs} {
				lines := strings.Split(stripAnsi(m.View()), "\n")
				line := lines[mgmtTabStripLine]
				// The sidebar also has an Agents label; the main tab is last.
				idx := strings.LastIndex(line, target.label())
				if idx < 0 {
					t.Fatalf("width %d: missing %s in %q", width, target.label(), line)
				}
				x := lipgloss.Width(line[:idx])
				got, _ := m.Update(leftClick(x, mgmtTabStripLine))
				m = got.(model)
				if m.jobs.main != target {
					t.Fatalf("width %d, sidebar %v: click on visible %s at %d failed", width, showTree, target.label(), x)
				}
			}
		}
	}
}

func TestAgentsHideSessionStripAndIgnoreItsClicks(t *testing.T) {
	m := agentsViewModel()
	l := m.jobsLayout()
	_, spans := m.jobsSessionStrip(l.listW - 2)
	for _, sp := range spans {
		got, _ := m.handleMouse(leftClick(l.listX+1+sp.start, 0))
		if got.(model).jobs.session != m.jobs.session {
			t.Fatal("hidden session strip changed the session scope")
		}
	}
	lines := m.renderListPanel(l.listW, l.paneH, false)
	if strings.Contains(stripAnsi(lines[0]), "All") {
		t.Fatal("Agents still displays the session strip")
	}
}

// An empty list says what would fill it, rather than reading as a broken pane.
func TestAgentViewEmptyStateOffersAnAction(t *testing.T) {
	m := agentsViewModel()
	m.agents = nil
	m.refreshTreeRows()
	got := agentViewText(m, 70, 8)
	if !strings.Contains(got, "no live agents") {
		t.Errorf("expected an explanation:\n%s", got)
	}
	if !strings.Contains(got, "new session") {
		t.Errorf("expected an offered action:\n%s", got)
	}
}

// A stalled agent reads as a doubt, not a failure: it says how long it has been
// silent rather than branding a slow job broken.
func TestAgentViewReportsSilence(t *testing.T) {
	m := agentsViewModel()
	got := agentViewText(m, 100, 12)
	if !strings.Contains(got, "QUIET") {
		t.Errorf("the stalled agent should be marked quiet:\n%s", got)
	}
	if !strings.Contains(got, "no output since") {
		t.Errorf("its second line should say how long:\n%s", got)
	}
	if strings.Contains(got, "ERROR  ") && !strings.Contains(got, "QUIET") {
		t.Error("silence must not be reported as a failure")
	}
}

func TestMainTabPositionsStayFixed(t *testing.T) {
	for _, width := range []int{30, 80} {
		m := agentsViewModel()
		_, _, baseline := m.renderMainTabStrip(width)
		for _, count := range []int{0, 9, 10, 99, 100, 1000} {
			m.jobs.rows = make([]mgmtRow, count)
			for _, view := range []mainView{mainJobs, mainAgents} {
				m.jobs.main = view
				for _, attention := range []protocol.Attention{protocol.AttentionWorking, protocol.AttentionWaiting} {
					for i := range m.agents {
						m.agents[i].Attention = attention
					}
					strip, _, spans := m.renderMainTabStrip(width)
					for i, sp := range spans {
						if sp != baseline[i] {
							t.Fatalf("width %d, count %d, view %v: tab moved: %+v -> %+v", width, count, view, baseline[i], sp)
						}
						if !strings.Contains(stripAnsi(strip), sp.view.label()) {
							t.Fatal("primary label clipped")
						}
					}
				}
			}
		}
	}
}

func TestManagementFocusVisibleWithoutColor(t *testing.T) {
	for _, view := range []mainView{mainJobs, mainAgents} {
		m := agentsViewModel()
		m.jobs.main = view
		for _, treeFocus := range []bool{false, true} {
			m.jobs.tree.focus = treeFocus
			l := m.jobsLayout()
			tree := stripAnsi(m.renderTreePanel(l.treeW, l.paneH)[0])
			main := stripAnsi(m.renderListPanel(l.listW, l.listH, false)[0])
			if strings.HasPrefix(tree, "┏") != treeFocus || strings.HasPrefix(main, "┏") == treeFocus {
				t.Fatalf("focus %v: tree %q main %q", treeFocus, tree, main)
			}
		}
	}
}

func TestAgentCompletionPreview(t *testing.T) {
	m := agentsViewModel()
	m.agents[0].Attention = protocol.AttentionIdle
	m.agents[0].Note = "Fixed mouse navigation.\n\nTests passed.\tUpdated 3 files."
	text := agentViewText(m, 100, 12)
	if !strings.Contains(text, "Result: Fixed mouse navigation. Tests passed. Updated 3 files.") {
		t.Fatalf("missing completion preview:\n%s", text)
	}
	for _, width := range []int{24, 40, 100} {
		for _, row := range m.agentViewRows() {
			line := m.agentViewDetailLine(row, true, width)
			if strings.ContainsAny(line, "\n\r\t") || lipgloss.Width(line) != width {
				t.Fatalf("preview breaks row layout at width %d: %q", width, line)
			}
		}
	}
	// A new working report replaces the prior completion note.
	m.agents[0].Attention = protocol.AttentionWorking
	m.agents[0].Note = ""
	if strings.Contains(agentViewText(m, 100, 12), "Fixed mouse navigation") {
		t.Fatal("previous result leaked into the next task")
	}
}
