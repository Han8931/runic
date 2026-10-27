package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/han/runic/internal/protocol"
)

// sideTab is one view of the sidebar, shown as a tab on the Sessions pane and
// rolled with Tab / Shift+Tab while the tree has focus. The two views answer
// different questions: Spaces is "how is my work organized", Agents is "who
// needs me". They are the same data seen from two directions, so neither tab
// hides anything the other shows — Agents just drops the hierarchy and sorts by
// urgency instead of by name.
type sideTab int

const (
	// Spaces leads because it is the view that is never empty: a fresh daemon
	// has a default session and no agents at all, and a sidebar that opens on
	// an empty pane teaches nothing. The Agents tab carries a count, so it
	// advertises itself as soon as there is anything to see.
	sideSpaces sideTab = iota
	sideAgents
	numSideTabs
)

func (t sideTab) label() string {
	if t == sideAgents {
		return "Agents"
	}
	return "Spaces"
}

// cycleSideTab rolls the sidebar delta tabs left or right, wrapping around.
func (m *model) cycleSideTab(delta int) {
	n := int(numSideTabs)
	m.switchSideTab(sideTab(((int(m.jobs.tree.tab)+delta)%n + n) % n))
}

// switchSideTab changes the sidebar view and parks the cursor at the top. The
// row sets are unrelated between the two tabs — one is a fold tree, the other a
// flat urgency list — so there is no row to anchor to the way the job tabs do.
func (m *model) switchSideTab(t sideTab) {
	if t == m.jobs.tree.tab || t < 0 || t >= numSideTabs {
		return
	}
	m.jobs.tree.tab = t
	m.jobs.tree.cursor = 0
	m.jobs.tree.rows = nil
	m.jobs.tree.menu = treeMenu{}
	m.refreshTreeRows()
}

// sideTabCount is the number the tab advertises: live agents for Agents, spaces
// for Spaces.
func (m model) sideTabCount(t sideTab) int {
	if t == sideAgents {
		return len(m.agentRows())
	}
	return len(m.nodes)
}

// needsYouCount is how many live panes are blocked on you or stopped on an
// error. It is the one number the sidebar exists to show, so it stays visible
// on the Agents tab from either view.
func (m model) needsYouCount() int {
	n := 0
	for _, a := range m.agents {
		if protocol.Unresolved(a.Attention, a.Seen) {
			n++
		}
	}
	return n
}

// agentRows lists every live pane as an agent row, most urgent first: the panes
// that need you, then the ones working, then the quiet ones. Within a bucket the
// oldest state sorts first — an agent blocked for twenty minutes is a different
// problem from one that just stopped — and session/pane name breaks ties so the
// list never reshuffles under the cursor between polls.
func (m model) agentRows() []treeRow {
	rows := make([]treeRow, 0, len(m.agents))
	for _, a := range m.agents {
		// Only panes that have reported a state are agents. `ru mark` is how a
		// process identifies itself as one, so a plain shell split off for a
		// quick `git status` stays out of a list that claims to be agents.
		if a.Attention == protocol.AttentionNone {
			continue
		}
		rows = append(rows, treeRow{
			kind:      treeAgent,
			group:     m.sessionGroup(a.Session),
			session:   a.Session,
			pane:      a.Pane,
			attention: a.Attention,
			note:      a.Note,
			since:     a.Since,
			seen:      a.Seen,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if ra, rb := attentionRank(a.attention, a.seen), attentionRank(b.attention, b.seen); ra != rb {
			return ra < rb
		}
		// Zero Since (never reported) sorts last within its bucket; it carries
		// no age to compare.
		az, bz := a.since.IsZero(), b.since.IsZero()
		if az != bz {
			return bz
		}
		if !az && !a.since.Equal(b.since) {
			return a.since.Before(b.since)
		}
		if a.session != b.session {
			return a.session < b.session
		}
		return a.pane < b.pane
	})
	return rows
}

// attentionRank orders the states by how much they want your attention.
//
// An acknowledged error drops below the unresolved alerts — you have seen it,
// so it should not outrank something you have not — but stays above a working
// agent, because it is still a broken thing waiting to be dealt with and a
// working agent is healthy.
func attentionRank(a protocol.Attention, seen bool) int {
	switch {
	case a == protocol.AttentionError && !seen:
		return 0
	case a == protocol.AttentionWaiting && !seen:
		return 1
	case a == protocol.AttentionError:
		return 2 // acknowledged, still broken
	case a == protocol.AttentionWorking:
		return 3
	case a == protocol.AttentionIdle:
		return 4
	default:
		return 5
	}
}

// sessionGroup finds the group a session belongs to, so an agent row can carry
// it for the forms and the detail pane.
func (m model) sessionGroup(session string) string {
	for _, n := range m.nodes {
		for _, s := range n.sessions {
			if s.Name == session {
				return n.group
			}
		}
	}
	return ""
}

// sessionInfo looks up a session by name from the last tree poll.
func (m model) sessionInfo(session string) (protocol.SessionInfo, bool) {
	for _, n := range m.nodes {
		for _, s := range n.sessions {
			if s.Name == session {
				return s, true
			}
		}
	}
	return protocol.SessionInfo{}, false
}

// agentEmptyMessage tells the user why the Agents tab is empty and how it fills.
func agentEmptyMessage() (headline, hint string) {
	return "no live agents", "open a session, then  ru mark working"
}

// sideTabSpan is where one sidebar tab landed in the rendered strip, in columns
// relative to the strip's left edge, for mouse hit-testing.
type sideTabSpan struct {
	tab        sideTab
	start, end int // [start, end)
}

// renderSideTabStrip draws the sidebar header as tabs, matching the Jobs pane:
// the active tab is a filled chip carrying its count, the other is a plain name.
// The Agents tab shows its "needs you" count in amber even when inactive,
// because a blocked agent is worth noticing from the Spaces view.
func (m model) renderSideTabStrip(width int) (string, []sideTabSpan) {
	var b strings.Builder
	var spans []sideTabSpan
	col := 0
	write := func(s string) {
		b.WriteString(s)
		col += lipgloss.Width(s)
	}
	write(" ")
	for t := sideSpaces; t < numSideTabs; t++ {
		if t > sideSpaces {
			write("  ")
		}
		start := col
		switch {
		case t == m.jobs.tree.tab:
			write(chipStyle.Render(fmt.Sprintf(" %s %d ", t.label(), m.sideTabCount(t))))
		case t == sideAgents:
			if n := m.needsYouCount(); n > 0 {
				write(tabInactiveStyle.Render(t.label()+" ") + attnWaitingStyle.Render(fmt.Sprintf("%d!", n)))
			} else {
				write(tabInactiveStyle.Render(t.label()))
			}
		default:
			write(tabInactiveStyle.Render(t.label()))
		}
		spans = append(spans, sideTabSpan{tab: t, start: start, end: col})
	}
	return fitToWidth(b.String(), width), spans
}

// agentLabel names an agent row: the session, plus the pane when the session
// runs more than one, since a bare pane id means nothing on its own.
func (m model) agentLabel(r treeRow) string {
	if m.sessionPaneCount(r.session) > 1 {
		return r.session + " · " + r.pane
	}
	return r.session
}

// agentTreeLabel names an agent sitting under its own session in the Spaces
// tree. The session name is already on the row above, so repeating it would
// waste the width twice over; what the row adds is *which agent this is* — its
// kind, and its pane when the session runs more than one.
func (m model) agentTreeLabel(r treeRow) string {
	label := r.agentKind
	if label == "" {
		label = "agent"
	}
	if m.sessionPaneCount(r.session) > 1 {
		label += " " + r.pane
	}
	return label
}

// sessionAgentRows splits a session's agents into the ones that want something
// from you and the ones that do not, so the tree can place them on either side
// of the session's jobs.
func (m model) sessionAgentRows(group, session string) (urgent, calm []treeRow) {
	kind := ""
	if s, ok := m.sessionInfo(session); ok {
		kind = s.Agent
	}
	for _, r := range m.agentRows() {
		if r.session != session {
			continue
		}
		r.group = group
		r.agentKind = kind
		if protocol.Unresolved(r.attention, r.seen) {
			urgent = append(urgent, r)
		} else {
			calm = append(calm, r)
		}
	}
	return urgent, calm
}

// sessionPaneCount is how many live panes a session has.
func (m model) sessionPaneCount(session string) int {
	n := 0
	for _, a := range m.agents {
		if a.Session == session {
			n++
		}
	}
	return n
}

// agentChip is the right-hand status column on an agent row: the age of a state
// that is waiting on you, and nothing at all for a quiet pane.
func (m model) agentChip(r treeRow) (string, lipgloss.Style) {
	badge, style, reported := attentionBadge(r.attention, r.seen, m.spinner())
	if !reported {
		return "", treeSummaryStyle
	}
	if r.since.IsZero() {
		return badge, style
	}
	// The age belongs next to the state it is the age *of*. A working agent
	// reports it quietly — useful for spotting one that is stuck, not a thing
	// to interrupt for — while anything unresolved keeps its own color.
	if r.attention == protocol.AttentionWorking {
		return badge + " " + waitAge(r.since), style
	}
	return badge + " " + waitAge(r.since), style
}

// agentStateWord spells out an agent's state for the detail pane, where there is
// room for a word instead of a glyph.
func agentStateWord(a protocol.Attention, seen bool) string {
	if a == protocol.AttentionNone {
		return "no report"
	}
	if seen && a == protocol.AttentionError {
		// Name both halves: the failure stands, and you have already seen it.
		// Without the second half, an error that has stopped nagging looks like
		// a bug in the inbox.
		return "error (seen — still unresolved)"
	}
	return a.String()
}
