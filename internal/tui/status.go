package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// The split view's status bar, in the same gorae style as the management
// screen's: mode chip, LABEL value pairs, and key caps on the right.

func (m model) renderFooter() string {
	line, _ := m.footerSpans()
	return line
}

// footerSpans renders the split view's status bar and reports where its key caps
// landed, so the same function answers both the renderer and a click. The caps
// are right-aligned, so their columns depend on the whole line's layout — which
// is exactly why this is computed in one place rather than guessed at.
func (m model) footerSpans() (string, []hintSpan) {
	val := func(s string) string { return statusValueStyle.Render(" " + s + " ") }
	left := m.modeStyle().Render(" "+m.modeName()+" ") + statusPair("SESSION", val(m.activeSession))
	// The pane counter earns its place in a tiling view: it is the only way to
	// tell "this session has one pane" from "the others are off-screen".
	if n := m.paneCount(); n > 1 {
		left += statusPair("PANE", val(fmt.Sprintf("%d/%d", m.focusedPaneIndex()+1, n)))
	}
	switch {
	case strings.HasPrefix(m.status, "error"):
		left += " " + dangerChipStyle.Render(" ERR ") + val(m.status)
	case m.status != "":
		left += statusPair("MSG", val(m.status))
	}
	// An agent blocking in *another* session is the case this exists for: the
	// tab strip marks it with a glyph, and this says what it actually wants.
	if !m.alarm.alert.empty() {
		left += " " + dangerChipStyle.Render(" ! "+shorten(m.alarm.alert.text(), 44)+" ")
	}

	right := ""
	if m.gitBranch != "" {
		right += statusPair("BRANCH", val("⊕ "+m.gitBranch))
	}
	right += statusPair("JOBS", val(fmt.Sprint(m.jobCount()))) +
		statusPair("SLOTS", val(fmt.Sprint(m.maxSlots))) + " "
	// The tab chord is only worth advertising when there is somewhere to go;
	// the tabs on the top border are otherwise the only hint that it exists.
	// Detach leads: it is the way out of a session, it was previously reachable
	// only by knowing the chord, and it is the one thing you cannot discover by
	// looking at the screen.
	hints := []keyHint{{"^B d", "detach"}, {"^B s", "sessions"}, {"^B j", "jobs"}, {"^B |/-", "split"}}
	if m.jobs.tree.focus {
		hints = []keyHint{{"enter", "open"}, {"n", "new"}, {"e", "edit/move"}, {"d", "delete"}, {"tab", "agents"}, {"esc", "terminal"}}
	}
	// Gated on the same list the chord walks (sessionNames, the daemon's whole
	// catalog) — not just the sessions already open, or the hint would hide
	// while the tabs on the border plainly offer somewhere to go.
	if len(m.sessionNames()) > 1 {
		hints = append([]keyHint{{"^B n/p", "session"}}, hints...)
	}
	keys, spans := statusKeys(hints)
	right += keys
	line := renderStatusBar(m.width, left, right)
	// renderStatusBar drops the right side wholesale when it will not fit, which
	// takes the targets with it: reporting spans for caps that were not drawn
	// would make a click land on whatever is actually there.
	rightW := lipgloss.Width(right)
	if lipgloss.Width(left)+rightW > m.width {
		return line, nil
	}
	// The caps sit at the far right, after `keys` own offset within `right`.
	base := m.width - rightW + (rightW - lipgloss.Width(keys))
	for i := range spans {
		spans[i].start += base
		spans[i].end += base
	}
	return line, spans
}

// statusKeys renders key caps for the status bar's right edge, with the columns
// each one occupies relative to the start of the returned string.
func statusKeys(hints []keyHint) (string, []hintSpan) {
	var parts []string
	var spans []hintSpan
	sep := statusBarStyle.Render(" ")
	sepW := lipgloss.Width(sep)
	col := 0
	for i, h := range hints {
		chip := keyCapStyle.Render(" "+h.key+" ") + statusValueStyle.Foreground(cFgDim).Render(" "+h.label+" ")
		if i > 0 {
			col += sepW
		}
		w := lipgloss.Width(chip)
		spans = append(spans, hintSpan{hint: h, start: col, end: col + w})
		col += w
		parts = append(parts, chip)
	}
	return strings.Join(parts, sep), spans
}

// statusPair renders a gorae-style LABEL value pair for a status bar.
func statusPair(label, value string) string {
	return statusLabelStyle.Render(" "+label) + value
}

// renderStatusBar lays left and right out on one full-width bar. When both
// don't fit, the right side goes first: the mode and message matter more.
func renderStatusBar(width int, left, right string) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(left)+lipgloss.Width(right) > width {
		right = ""
	}
	pad := max(0, width-lipgloss.Width(left)-lipgloss.Width(right))
	return fitToWidth(left+statusBarStyle.Render(strings.Repeat(" ", pad))+right, width)
}

// modeName reports the current editing mode in vim terms: COMMAND while a TUI
// command prompt is open, otherwise INSERT (typing into the focused shell pane).
func (m model) modeName() string {
	if m.inputMode != inputNone {
		return "COMMAND"
	}
	return "INSERT"
}

func (m model) modeStyle() lipgloss.Style {
	if m.inputMode != inputNone {
		return modeCommandStyle
	}
	return modeInsertStyle
}

// focusedPaneIndex is the focused pane's position in the active session's
// traversal order, or 0 when there is nothing focused.
func (m model) focusedPaneIndex() int {
	root := m.activeRoot()
	if root == nil || m.focusPane == nil {
		return 0
	}
	for i, leaf := range root.leaves() {
		if leaf == m.focusPane {
			return i
		}
	}
	return 0
}

func (m model) jobCount() int {
	total := 0
	for _, node := range m.nodes {
		for _, session := range node.sessions {
			total += len(node.jobs[session.Name])
		}
	}
	return total
}
