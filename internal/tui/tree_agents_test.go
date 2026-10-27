package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/han/runic/internal/protocol"
)

// The Spaces tree carries each session's agents as its children: the ones that
// need you above the jobs, the quiet ones below. An agent you have to answer is
// the one child worth surfacing; one that is working away belongs out of the
// way until it has something to say.
func TestSpacesTreeNestsAgents(t *testing.T) {
	m := sideTabsModel()
	m.jobs.tree.tab = sideSpaces
	m.refreshTreeRows()

	// Walk the rows under session "build": p1 is waiting, p2 has errored, and
	// the session has one running job.
	var seen []string
	inBuild := false
	for _, r := range m.jobs.tree.rows {
		switch {
		case r.kind == treeSession && r.session == "build":
			inBuild = true
		case r.kind == treeSession || r.kind == treeGroup:
			inBuild = false
		case inBuild && r.kind == treeAgent:
			seen = append(seen, "agent:"+r.pane)
		case inBuild && r.kind == treeJob:
			seen = append(seen, "job")
		}
	}
	got := strings.Join(seen, ",")
	// Both of build's agents are unresolved, so both float above the job.
	if got != "agent:p1,agent:p2,job" && got != "agent:p2,agent:p1,job" {
		t.Errorf("rows under build = %q, want its blocked agents above the job", got)
	}
}

// A calm agent sits below the jobs — present, but not competing with them.
func TestCalmAgentSortsBelowJobs(t *testing.T) {
	m := sideTabsModel()
	// Quiet both of build's agents.
	for i := range m.agents {
		if m.agents[i].Session == "build" {
			m.agents[i].Attention = protocol.AttentionWorking
			m.agents[i].Seen = false
		}
	}
	m.jobs.tree.tab = sideSpaces
	m.refreshTreeRows()

	var order []string
	inBuild := false
	for _, r := range m.jobs.tree.rows {
		switch {
		case r.kind == treeSession && r.session == "build":
			inBuild = true
		case r.kind == treeSession || r.kind == treeGroup:
			inBuild = false
		case inBuild && r.kind == treeAgent:
			order = append(order, "agent")
		case inBuild && r.kind == treeJob:
			order = append(order, "job")
		}
	}
	if len(order) == 0 || order[0] != "job" {
		t.Errorf("rows under build = %v, want the job first when no agent needs you", order)
	}
}

// A session with an agent but no jobs still has to be expandable, or its agent
// is unreachable in the tree.
func TestSessionWithOnlyAgentsIsExpandable(t *testing.T) {
	m := sideTabsModel()
	m.jobs.allJobs = nil
	m.nodes = buildTree([]string{"default", "work"}, []protocol.SessionInfo{
		{Name: "build", Group: "work"},
	}, nil)
	m.jobs.tree.tab = sideSpaces
	m.refreshTreeRows()

	for _, r := range m.jobs.tree.rows {
		if r.kind == treeSession && r.session == "build" {
			if !r.hasKids {
				t.Error("a session hosting an agent should be expandable")
			}
			return
		}
	}
	t.Fatal("no build session row")
}

// An agent row must not look like a session row. The distinction is structural
// now that the geometric markers are gone: a session carries a fold mark and
// its own name, an agent sits a level deeper and is named by its kind.
func TestAgentRowsAreDistinctFromSessions(t *testing.T) {
	m := sideTabsModel()
	m.jobs.tree.tab = sideSpaces
	m.refreshTreeRows()

	var agentText, sessionText string
	for _, r := range m.jobs.tree.rows {
		line := stripAnsi(m.renderTreeRow(r, false, treePaneWidth))
		switch {
		case r.kind == treeAgent && agentText == "":
			agentText = line
		case r.kind == treeSession && r.session == "build" && sessionText == "":
			sessionText = line
		}
	}
	if agentText == "" || sessionText == "" {
		t.Fatal("expected both an agent row and a session row")
	}
	// The retired vocabulary must not come back.
	for _, glyph := range []string{"◆", "●", "◐", "×", "⊕"} {
		for _, line := range []string{agentText, sessionText} {
			if strings.Contains(line, glyph) {
				t.Errorf("%q is a retired glyph, found in %q", glyph, line)
			}
		}
	}
	// The agent names what it is; the session names itself.
	if !strings.Contains(agentText, "agent") && !strings.Contains(agentText, "claude") {
		t.Errorf("an agent row should be named by its kind: %q", agentText)
	}
	if !strings.Contains(sessionText, "build") {
		t.Errorf("a session row should carry its name: %q", sessionText)
	}
	// And it sits deeper than its session.
	if indent(agentText) <= indent(sessionText) {
		t.Errorf("the agent row should be indented under its session:\n%q\n%q", sessionText, agentText)
	}
}

// indent counts leading spaces after the cursor gutter.
func indent(line string) int {
	n := 0
	for _, r := range line[1:] {
		if r != ' ' {
			break
		}
		n++
	}
	return n
}

// Color is spent on urgency alone: a calm agent is faint like any other detail,
// and only one that needs you takes a loud hue. Giving "is an agent" its own
// color left nothing louder for "this agent is stuck".
func TestAgentRowColors(t *testing.T) {
	orig := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(orig)

	m := sideTabsModel()
	m.jobs.tree.tab = sideAgents
	m.refreshTreeRows()

	render := func(a protocol.Attention, seen bool) string {
		r := treeRow{kind: treeAgent, session: "s", pane: "p1", attention: a, seen: seen, since: time.Now()}
		return m.renderTreeRow(r, false, treePaneWidth)
	}
	// Compare on the color triple alone: the attention styles are also bold, so
	// an exact code match would fail for a reason that has nothing to do with
	// which color was chosen.
	rgb := func(c lipgloss.TerminalColor) string {
		codes := ansiCodes(lipgloss.NewStyle().Foreground(c).Render("x"))
		_, triple, _ := strings.Cut(codes, "38;2;")
		triple, _, _ = strings.Cut(triple, "m")
		return triple
	}
	cases := []struct {
		name      string
		attention protocol.Attention
		seen      bool
		want      lipgloss.TerminalColor
	}{
		{"working is faint — an agent, and calm", protocol.AttentionWorking, false, cFgFaint},
		{"waiting takes its attention color", protocol.AttentionWaiting, false, cAmber},
		{"errored is red", protocol.AttentionError, false, cRed},
		// Acknowledged: still true, no longer news, so it stops shouting.
		{"an acknowledged error goes quiet", protocol.AttentionError, true, cFgFaint},
		{"idle is faint too", protocol.AttentionIdle, false, cFgFaint},
	}
	for _, c := range cases {
		want := rgb(c.want)
		if got := ansiCodes(render(c.attention, c.seen)); !strings.Contains(got, want) {
			t.Errorf("%s: codes %q do not contain %q", c.name, got, want)
		}
	}
}

// Every agent row says its state in words, in both tabs. A badge survives a
// terminal with no color, which a hue alone does not.
func TestAgentRowsCarryAStateBadge(t *testing.T) {
	m := sideTabsModel()
	for _, tab := range []sideTab{sideAgents, sideSpaces} {
		m.jobs.tree.tab = tab
		m.refreshTreeRows()
		seen := map[string]bool{}
		for _, r := range m.jobs.tree.rows {
			if r.kind != treeAgent {
				continue
			}
			line := stripAnsi(m.renderTreeRow(r, false, treePaneWidth))
			badge, _, reported := attentionBadge(r.attention, r.seen, m.spinner())
			if reported && !strings.Contains(line, badge) {
				t.Errorf("tab %v: row %q should carry its badge %q", tab, line, badge)
			}
			seen[r.attention.String()] = true
		}
		if !seen["waiting"] || !seen["error"] {
			t.Fatalf("tab %v: expected the fixture to hold a waiting and an errored agent", tab)
		}
	}
}

// ansiCodes keeps only the escape sequences of a rendered string, so a test can
// assert on color without depending on the text around it.
func ansiCodes(s string) string {
	var b strings.Builder
	for {
		i := strings.Index(s, "\x1b[")
		if i < 0 {
			return b.String()
		}
		j := strings.IndexByte(s[i:], 'm')
		if j < 0 {
			return b.String()
		}
		b.WriteString(s[i : i+j+1])
		s = s[i+j+1:]
	}
}

// `!` acknowledges without attaching. Attaching is a heavy way to say "noted"
// — it takes over the screen — and on a waiting agent it is easy to mistake for
// having answered, which is why there has to be a way to say it from the list.
func TestAckFromTheSidebar(t *testing.T) {
	m := sideTabsModel()
	m.jobs.tree.tab = sideAgents
	m.refreshTreeRows()

	// The fixture's first row is the unresolved error.
	r, ok := m.jobs.tree.current()
	if !ok || !protocol.Unresolved(r.attention, r.seen) {
		t.Fatalf("expected an unresolved agent under the cursor, got %+v", r)
	}
	_, cmd, handled := m.handleTreeKey(keyRune('!'))
	if !handled {
		t.Fatal("the sidebar should handle !")
	}
	if cmd == nil {
		t.Fatal("! on an unresolved agent should send an acknowledgement")
	}

	// A row with nothing outstanding has nothing to acknowledge, so the key is
	// inert rather than firing a request that would be refused.
	for i, row := range m.jobs.tree.rows {
		if !protocol.Unresolved(row.attention, row.seen) {
			m.jobs.tree.cursor = i
			if _, cmd, _ := m.handleTreeKey(keyRune('!')); cmd != nil {
				t.Errorf("! on %v should do nothing", row.attention)
			}
			return
		}
	}
}
