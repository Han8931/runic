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

// The whole point of the change: an agent row must not look like a session row.
// Shape carries it, so this holds even where color does not survive.
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
	if !strings.Contains(agentText, "◆") {
		t.Errorf("an agent row should be marked ◆: %q", agentText)
	}
	if strings.Contains(sessionText, "◆") {
		t.Errorf("a session row must not wear the agent marker: %q", sessionText)
	}
	// The nested row names the agent, not the session it already sits under.
	if !strings.Contains(agentText, "claude") && !strings.Contains(agentText, "agent") {
		t.Errorf("an agent row should name its kind: %q", agentText)
	}
}

// Color backs the shape up: a calm agent is violet rather than the session
// foreground, and an agent needing you takes its attention color.
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
		{"working is violet — an agent, and calm", protocol.AttentionWorking, false, cViolet},
		{"waiting takes its attention color", protocol.AttentionWaiting, false, cAmber},
		{"errored is red", protocol.AttentionError, false, cRed},
		{"an acknowledged error stays red", protocol.AttentionError, true, cRed},
		{"idle is violet, quietly", protocol.AttentionIdle, false, cViolet},
	}
	for _, c := range cases {
		want := rgb(c.want)
		if got := ansiCodes(render(c.attention, c.seen)); !strings.Contains(got, want) {
			t.Errorf("%s: codes %q do not contain %q", c.name, got, want)
		}
	}
}

// The flat Agents tab keeps the state glyphs instead: every row there is an
// agent, so the column is better spent on something that survives a terminal
// with no color.
func TestFlatAgentTabKeepsStateGlyphs(t *testing.T) {
	m := sideTabsModel()
	m.jobs.tree.tab = sideAgents
	m.refreshTreeRows()

	var waiting, errored bool
	for _, r := range m.jobs.tree.rows {
		line := stripAnsi(m.renderTreeRow(r, false, treePaneWidth))
		if strings.Contains(line, "◆") {
			t.Errorf("the flat list should not spend a column on ◆: %q", line)
		}
		glyph, _, _ := attentionMark(r.attention)
		if glyph != "" && !strings.Contains(line, glyph) {
			t.Errorf("row %q should carry its state glyph %q", line, glyph)
		}
		switch r.attention {
		case protocol.AttentionWaiting:
			waiting = true
		case protocol.AttentionError:
			errored = true
		}
	}
	if !waiting || !errored {
		t.Fatal("expected the fixture to hold a waiting and an errored agent")
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
