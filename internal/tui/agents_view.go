package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/han/runic/internal/protocol"
)

// The Agents view: the main pane's other primary view, beside Jobs.
//
// Agents lived only in the 26-column sidebar, where a name, a state and an age
// already fill the row — so the one thing you actually need, the question the
// agent is blocked on, had nowhere to go. Here there is room for it on a second
// line, which is the whole point: a blocked agent should be answerable from the
// list rather than requiring a visit.

// mainView selects what the main pane shows. Jobs and Agents are the two
// primary objects runic manages; the job-state tabs are a filter *within* Jobs
// rather than a peer of it.
type mainView int

const (
	mainJobs mainView = iota
	mainAgents
	numMainViews
)

func (v mainView) label() string {
	if v == mainAgents {
		return "Agents"
	}
	return "Jobs"
}

// agentDensity is how much room a row gets.
type agentDensity int

const (
	// densityComfortable gives each agent two lines, the second carrying what it
	// asked. The default, because the request is the reason the row exists.
	densityComfortable agentDensity = iota
	// densityCompact is one line each, for triaging many agents at once.
	densityCompact
)

// agentViewRows is the list the main Agents view shows, in the same urgency
// order as the sidebar's — one ordering for "who needs me", wherever it is
// asked.
func (m model) agentViewRows() []treeRow {
	return m.agentRows()
}

// agentRowHeight is how many terminal rows one agent occupies.
func (m model) agentRowHeight() int {
	if m.jobs.density == densityCompact {
		return 1
	}
	return 2
}

// agentViewCapacity is how many agents fit in the pane body.
func (m model) agentViewCapacity(bodyH int) int {
	if h := m.agentRowHeight(); h > 0 {
		return max(1, bodyH/h)
	}
	return 1
}

// renderAgentView draws the main Agents view into exactly bodyH lines of inner
// width.
//
// Columns are budgeted the way the job table's are — what the row *is* survives,
// the scope columns are added only when they fit — because the same narrow
// terminal that starves the job table starves this one.
func (m model) renderAgentView(inner, bodyH int) []string {
	rows := m.agentViewRows()
	if len(rows) == 0 {
		return m.agentViewEmpty(inner, bodyH)
	}

	// NAME and STATE always survive. KIND and BRANCH are extras.
	nameW := max(12, min(24, inner/3))
	stateW := 12
	kindW, branchW := 0, 0
	if rest := inner - nameW - stateW - 6; rest >= 10 {
		kindW = 8
		if rest-kindW >= 14 {
			branchW = min(18, rest-kindW)
		}
	}

	out := make([]string, 0, bodyH)
	cap := m.agentViewCapacity(bodyH)
	offset := clampOffset(m.jobs.agentCursor, m.jobs.agentOffset, cap, len(rows))
	for i := offset; i < len(rows) && len(out) < bodyH; i++ {
		r := rows[i]
		selected := i == m.jobs.agentCursor
		out = append(out, m.agentViewLine(r, selected, inner, nameW, stateW, kindW, branchW))
		if m.jobs.density == densityComfortable && len(out) < bodyH {
			out = append(out, m.agentViewDetailLine(r, selected, inner))
		}
	}
	for len(out) < bodyH {
		out = append(out, fitToWidth("", inner))
	}
	return out[:bodyH]
}

// agentViewLine is an agent's first line: who it is, what it is doing, and the
// provenance that fits.
func (m model) agentViewLine(r treeRow, selected bool, inner, nameW, stateW, kindW, branchW int) string {
	var bg lipgloss.TerminalColor
	if selected {
		bg = cRowFocusBg
	}
	st := func(s lipgloss.Style) lipgloss.Style {
		if bg != nil {
			return s.Background(bg)
		}
		return s
	}
	cell := func(text string, w int, style lipgloss.Style) string {
		return st(style).Render(fitToWidth(stripAnsi(text), w))
	}

	// The marker column: an exclamation for anything still wanting you. One
	// column, and the only thing on the line that is allowed to shout.
	mark, markStyle := " ", treeSummaryStyle
	if protocol.Unresolved(r.attention, r.seen) {
		_, markStyle, _ = attentionBadge(r.attention, r.seen, m.spinner())
		mark = "!"
	}

	badge, badgeStyle, _ := attentionBadge(r.attention, r.seen, m.spinner())
	state := strings.ToUpper(agentStateWord(r.attention, r.seen))
	if agentStalled(r) {
		state, badgeStyle = "QUIET", attnWaitingStyle
	} else if r.attention == protocol.AttentionWorking {
		// The spinner carries "working" better than the word does.
		state = badge + " WORKING"
	}
	age := ""
	if !r.since.IsZero() && r.attention != protocol.AttentionNone {
		age = waitAge(r.since)
	}
	if agentStalled(r) {
		age = waitAge(r.lastOutput)
	}

	parts := []string{
		st(markStyle).Render(mark),
		cell(m.agentLabel(r), nameW, sessionStyle),
		cell(state, stateW, badgeStyle),
		cell(age, 5, treeSummaryStyle),
	}
	if kindW > 0 {
		kind := ""
		if s, ok := m.sessionInfo(r.session); ok {
			kind = s.Agent
		}
		parts = append(parts, cell(kind, kindW, treeSummaryStyle))
	}
	if branchW > 0 {
		branch := ""
		if s, ok := m.sessionInfo(r.session); ok {
			branch = s.Branch
		}
		parts = append(parts, cell(branch, branchW, treeSummaryStyle))
	}
	sep := " "
	if bg != nil {
		sep = st(lipgloss.NewStyle()).Render(" ")
	}
	return padRowBg(fitToWidth(strings.Join(parts, sep), inner), inner, bg)
}

// agentViewDetailLine is the second line: what the agent asked, or what it last
// printed when it asked nothing. This is the line the sidebar had no room for,
// and the reason a two-line row is the default.
func (m model) agentViewDetailLine(r treeRow, selected bool, inner int) string {
	var bg lipgloss.TerminalColor
	if selected {
		bg = cRowFocusBg
	}
	style := treeEmptyStyle
	text := r.note
	switch {
	case text != "":
		if protocol.Unresolved(r.attention, r.seen) {
			// Its own words, quoted, because that is what you are answering.
			text = "“" + text + "”"
			style = treeSummaryStyle
		}
	case agentStalled(r):
		text = "no output since " + waitAge(r.lastOutput) + " ago"
	case !r.lastOutput.IsZero():
		// Liveness, not the branch: the branch already has its own column, and
		// how recently it said anything is the thing you cannot see elsewhere.
		text = "last output " + waitAge(r.lastOutput) + " ago"
	}
	if text == "" {
		text = "—"
	}
	if bg != nil {
		style = style.Background(bg)
	}
	line := fitToWidth("   "+style.Render(truncateToWidth(stripAnsi(text), max(8, inner-4))), inner)
	return padRowBg(line, inner, bg)
}

// agentViewEmpty says why the view is empty and what would fill it. An empty
// list that explains nothing reads as a broken screen.
func (m model) agentViewEmpty(inner, bodyH int) []string {
	headline, hint := agentEmptyMessage()
	out := []string{
		fitToWidth("", inner),
		fitToWidth("  "+treeEmptyStyle.Render(headline), inner),
		fitToWidth("  "+helpStyle.Render(hint), inner),
		fitToWidth("", inner),
		fitToWidth("  "+helpStyle.Render("n  new session — pick an agent in the box"), inner),
	}
	for len(out) < bodyH {
		out = append(out, fitToWidth("", inner))
	}
	return out[:bodyH]
}

// agentViewHeader labels the columns, matching what renderAgentView drew.
func (m model) agentViewHeader(inner int) string {
	nameW := max(12, min(24, inner/3))
	stateW := 12
	kindW, branchW := 0, 0
	if rest := inner - nameW - stateW - 6; rest >= 10 {
		kindW = 8
		if rest-kindW >= 14 {
			branchW = min(18, rest-kindW)
		}
	}
	cells := []string{
		" ",
		fitToWidth("AGENT", nameW),
		fitToWidth("STATE", stateW),
		fitToWidth("FOR", 5),
	}
	if kindW > 0 {
		cells = append(cells, fitToWidth("KIND", kindW))
	}
	if branchW > 0 {
		cells = append(cells, fitToWidth("BRANCH", branchW))
	}
	return jobsHeaderStyle.Render(fitToWidth(strings.Join(cells, " "), inner))
}

// agentCursorRow is the agent under the cursor in the main view.
func (m model) agentCursorRow() (treeRow, bool) {
	rows := m.agentViewRows()
	if m.jobs.agentCursor < 0 || m.jobs.agentCursor >= len(rows) {
		return treeRow{}, false
	}
	return rows[m.jobs.agentCursor], true
}

// moveAgentCursor walks the main Agents view, clamped.
func (m *model) moveAgentCursor(delta int) {
	rows := m.agentViewRows()
	if len(rows) == 0 {
		m.jobs.agentCursor = 0
		return
	}
	m.jobs.agentCursor = max(0, min(len(rows)-1, m.jobs.agentCursor+delta))
}

// agentViewHints are the keys the Agents view answers to.
func (m model) agentViewHints() []keyHint {
	density := "compact"
	if m.jobs.density == densityCompact {
		density = "expand"
	}
	hints := []keyHint{
		{key: "j/k", label: "move"},
		{key: "⏎", label: "attach"},
	}
	if r, ok := m.agentCursorRow(); ok && protocol.Unresolved(r.attention, r.seen) {
		hints = append(hints, keyHint{key: "!", label: "acknowledge"})
	}
	return append(hints,
		keyHint{key: "c", label: density},
		keyHint{key: "tab", label: "jobs"},
		keyHint{key: "e", label: "edit"},
		keyHint{key: "n", label: "new"},
		keyHint{key: "?", label: "help", keep: true},
		keyHint{key: "q", label: "quit", keep: true},
	)
}

// agentViewSummary is the count the primary tab advertises: how many agents
// there are, and how many of them want something.
func (m model) agentViewSummary() (total, needsYou int) {
	return len(m.agentViewRows()), m.needsYouCount()
}

// renderMainTabStrip draws the primary view tabs — Agents and Jobs — followed,
// when Jobs is showing, by its state filters. One strip rather than two rows:
// the filters belong *to* Jobs, so they read as its sub-navigation, and nothing
// shifts position when you switch views.
func (m model) renderMainTabStrip(width int) (string, []tabSpan, []mainTabSpan) {
	var b strings.Builder
	var mains []mainTabSpan
	col := 0
	write := func(s string) {
		b.WriteString(s)
		col += lipgloss.Width(s)
	}
	write(" ")
	for v := mainJobs; v < numMainViews; v++ {
		if v > mainJobs {
			write("  ")
		}
		start := col
		switch {
		case v == m.jobs.main && v == mainAgents:
			total, needs := m.agentViewSummary()
			label := fmt.Sprintf(" %s %d ", v.label(), total)
			if needs > 0 {
				label = fmt.Sprintf(" %s %d! ", v.label(), needs)
			}
			write(chipStyle.Render(label))
		case v == m.jobs.main:
			write(chipStyle.Render(fmt.Sprintf(" %s %d ", v.label(), len(m.jobs.rows))))
		case v == mainAgents:
			// A blocked agent has to be visible from the Jobs view, or the tab
			// is only useful once you already suspect something.
			if _, needs := m.agentViewSummary(); needs > 0 {
				write(tabInactiveStyle.Render(v.label()+" ") + attnWaitingStyle.Render(fmt.Sprintf("%d!", needs)))
			} else {
				write(tabInactiveStyle.Render(v.label()))
			}
		default:
			write(tabInactiveStyle.Render(v.label()))
		}
		mains = append(mains, mainTabSpan{view: v, start: start, end: col})
	}

	var jobSpans []tabSpan
	if m.jobs.main == mainJobs {
		write(jobsDetailRuleStyle.Render("  │ "))
		for t := tabAll; t < numJobsTabs; t++ {
			if t > tabAll {
				write("  ")
			}
			start := col
			switch {
			case t == m.jobs.tab:
				write(selectedStyle.Render(fmt.Sprintf(" %s %d ", t.label(), m.tabCount(t))))
			case t == tabFailed:
				if n := m.tabCount(t); n > 0 {
					write(tabInactiveStyle.Render(t.label()+" ") + finishedErrStyle.Render(fmt.Sprint(n)))
				} else {
					write(tabInactiveStyle.Render(t.label()))
				}
			default:
				write(tabInactiveStyle.Render(t.label()))
			}
			jobSpans = append(jobSpans, tabSpan{tab: t, start: start, end: col})
		}
	}
	return fitToWidth(b.String(), width), jobSpans, mains
}

// mainTabSpan is where a primary view tab landed, for mouse hit-testing.
type mainTabSpan struct {
	view       mainView
	start, end int
}

// switchMainView changes what the main pane shows.
func (m *model) switchMainView(v mainView) {
	if v < 0 || v >= numMainViews || v == m.jobs.main {
		return
	}
	m.jobs.main = v
	if v == mainAgents {
		m.jobs.agentCursor = 0
		m.jobs.agentOffset = 0
	}
}

// cycleMainView rolls between Agents and Jobs.
func (m *model) cycleMainView(delta int) {
	n := int(numMainViews)
	m.switchMainView(mainView(((int(m.jobs.main)+delta)%n + n) % n))
}

// handleAgentViewKey drives the main Agents view. The bool reports whether the
// key was consumed, so anything it does not claim falls through to the shared
// management bindings.
func (m model) handleAgentViewKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "j", "down":
		(&m).moveAgentCursor(1)
		return m, nil, true
	case "k", "up":
		(&m).moveAgentCursor(-1)
		return m, nil, true
	case "ctrl+d":
		(&m).moveAgentCursor(max(1, m.agentViewCapacity(m.jobsBodyHeight())/2))
		return m, nil, true
	case "ctrl+u":
		(&m).moveAgentCursor(-max(1, m.agentViewCapacity(m.jobsBodyHeight())/2))
		return m, nil, true
	case "G":
		(&m).moveAgentCursor(len(m.agentViewRows()))
		return m, nil, true
	case "c":
		// Density. Two lines is the default because the request is the point;
		// one line is for triaging a screenful of agents at once.
		if m.jobs.density == densityCompact {
			m.jobs.density = densityComfortable
		} else {
			m.jobs.density = densityCompact
		}
		return m, nil, true
	case "enter":
		if r, ok := m.agentCursorRow(); ok {
			nm, cmd := m.openSessionPane(r.session, r.pane)
			return nm, cmd, true
		}
		return m, nil, true
	case "!":
		if r, ok := m.agentCursorRow(); ok && protocol.Unresolved(r.attention, r.seen) {
			return m, ackAgent(r.session, r.pane), true
		}
		return m, nil, true
	case "e":
		// Editing an agent means editing the session that hosts it; there is
		// nothing else about a pane to edit.
		if r, ok := m.agentCursorRow(); ok {
			return m.openSessionForm(false, r.session, r.group), textinput.Blink, true
		}
		return m, nil, true
		// Deliberately not claimed: the job actions (x/u/r/d), which have nothing
		// to act on here, and `/`, `:`, `?`, `q`, which mean the same everywhere.
	}
	return m, nil, false
}
