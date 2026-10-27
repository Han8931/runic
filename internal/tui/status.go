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

	left := []barSegment{
		{text: m.modeStyle().Render(" " + m.modeName() + " ")}, // protected: where am I
		{text: statusPair("SESSION", val(m.activeSession)), drop: 3},
	}
	// The pane counter earns its place in a tiling view: it is the only way to
	// tell "this session has one pane" from "the others are off-screen".
	if n := m.paneCount(); n > 1 {
		left = append(left, barSegment{
			text: statusPair("PANE", val(fmt.Sprintf("%d/%d", m.focusedPaneIndex()+1, n))),
			drop: 4,
		})
	}
	switch {
	case strings.HasPrefix(m.status, "error"):
		left = append(left, barSegment{text: " " + dangerChipStyle.Render(" ERR ") + val(m.status), drop: 1})
	case m.status != "":
		left = append(left, barSegment{text: statusPair("MSG", val(m.status)), drop: 2})
	}
	if m.paneZoom {
		// Zoom hides the other panes, so the one thing that must be on screen is
		// the fact that they exist.
		left = append(left, barSegment{text: " " + warnChipStyle.Render(" ZOOM ")})
	}
	// An agent blocking in *another* session is the case the alarm exists for,
	// so it is never the thing that gets dropped.
	if !m.alarm.alert.empty() {
		left = append(left, barSegment{
			text: " " + dangerChipStyle.Render(" ! "+shorten(m.alarm.alert.text(), 44)+" "),
		})
	}

	// Getting out of a session, reaching the session list, and finding the help
	// are protected. Everything else here is a readout.
	out := []keyHint{{key: "^B d", label: "detach"}, {key: "^B j", label: "jobs"}, {key: "^B ?", label: "help"}}
	secondary := []keyHint{{key: "^B s", label: "sessions"}, {key: "^B |/-", label: "split"}}
	if m.jobs.tree.focus {
		out = []keyHint{{key: "enter", label: "open"}, {key: "esc", label: "terminal"}}
		secondary = []keyHint{{key: "n", label: "new"}, {key: "e", label: "edit/move"}, {key: "d", label: "delete"}, {key: "tab", label: "agents"}}
	}
	// Gated on the same list the chord walks (sessionNames, the daemon's whole
	// catalog) — not just the sessions already open, or the hint would hide
	// while the tabs on the border plainly offer somewhere to go.
	if len(m.sessionNames()) > 1 {
		secondary = append([]keyHint{{key: "^B n/p", label: "session"}}, secondary...)
	}
	outKeys, outSpans := statusKeys(out)
	secKeys, secSpans := statusKeys(secondary)

	right := []barSegment{
		{text: statusPair("JOBS", val(fmt.Sprint(m.jobCount()))), drop: 6},
		{text: statusPair("SLOTS", val(fmt.Sprint(m.maxSlots))), drop: 7},
		{text: " " + secKeys, drop: 4, keys: shiftSpans(secSpans, 1)},
		{text: " " + outKeys, keys: shiftSpans(outSpans, 1)},
	}
	if m.gitBranch != "" {
		// Which branch an agent is editing is worth more than a slot count or a
		// job tally, both of which the management screen also shows.
		right = append([]barSegment{{text: statusPair("BRANCH", val("⊕ "+m.gitBranch)), drop: 5}}, right...)
	}
	return fitBar(m.width, left, right)
}

// shiftSpans offsets hint spans by n columns, for a segment whose text is
// preceded by padding.
func shiftSpans(spans []hintSpan, n int) []hintSpan {
	out := make([]hintSpan, len(spans))
	for i, sp := range spans {
		sp.start += n
		sp.end += n
		out[i] = sp
	}
	return out
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

// barSegment is one piece of a status bar together with how readily it may be
// given up when the terminal is too narrow for all of it.
//
// drop 0 is protected and never dropped: the mode chip, a live alarm, and the
// keys that get you *out* of where you are. Everything else is ranked by how
// much is lost by its absence — a branch name and a slot count are nice, but
// losing the only on-screen evidence that detach exists is not a trade worth
// making, and that is exactly what dropping the whole right side used to do.
type barSegment struct {
	text string
	drop int
	// keys are hint spans within this segment, offset from its own start, so a
	// segment that survives the fit can still be clicked.
	keys []hintSpan
}

// fitBar lays left and right out on one full-width bar, shedding the most
// expendable segments until it fits rather than discarding a whole side.
func fitBar(width int, left, right []barSegment) (string, []hintSpan) {
	if width <= 0 {
		return "", nil
	}
	keep := func(segs []barSegment) []barSegment {
		out := make([]barSegment, 0, len(segs))
		for _, s := range segs {
			if s.text != "" {
				out = append(out, s)
			}
		}
		return out
	}
	left, right = keep(left), keep(right)

	total := func() int {
		n := 0
		for _, s := range left {
			n += lipgloss.Width(s.text)
		}
		for _, s := range right {
			n += lipgloss.Width(s.text)
		}
		return n
	}
	// Shed the highest-priority-to-drop segment until it fits. Ties go to the
	// later segment, so a bar sheds from its outer edge inward.
	for total() > width {
		worst, from, at := 0, (*[]barSegment)(nil), -1
		for i, s := range right {
			if s.drop >= worst && s.drop > 0 {
				worst, from, at = s.drop, &right, i
			}
		}
		for i, s := range left {
			if s.drop >= worst && s.drop > 0 {
				worst, from, at = s.drop, &left, i
			}
		}
		if from == nil {
			break // only protected segments remain; truncation below handles it
		}
		*from = append((*from)[:at], (*from)[at+1:]...)
	}

	var lb, rb strings.Builder
	for _, s := range left {
		lb.WriteString(s.text)
	}
	var spans []hintSpan
	offset := 0
	for _, s := range right {
		rb.WriteString(s.text)
		for _, sp := range s.keys {
			sp.start += offset
			sp.end += offset
			spans = append(spans, sp)
		}
		offset += lipgloss.Width(s.text)
	}
	l, r := lb.String(), rb.String()
	lw, rw := lipgloss.Width(l), lipgloss.Width(r)
	if lw+rw > width {
		// Nothing left to shed: keep the left edge, which holds the mode and the
		// message, and let the right go under the truncation.
		return fitToWidth(l+r, width), nil
	}
	base := width - rw
	for i := range spans {
		spans[i].start += base
		spans[i].end += base
	}
	return fitToWidth(l+statusBarStyle.Render(strings.Repeat(" ", width-lw-rw))+r, width), spans
}

// ctrlBHints are the chords the prefix menu lists, in the order they are worth
// knowing. Shown the moment Ctrl+B is pressed, so the terminal's controls are
// discoverable when you reach for them rather than only in the help page.
//
// They are keyHints in the `^B x` form, which means the clickable hint bar can
// dispatch them without knowing anything about chords.
func (m model) ctrlBHints() []keyHint {
	hints := []keyHint{
		{key: "^B d", label: "detach"},
		{key: "^B j", label: "jobs"},
		{key: "^B s", label: "sessions"},
	}
	// Ordered by how often you reach for them, since the tail is what gets
	// dropped on a narrow bar. Zoom and pane-switching beat splitting: you
	// split once and then live with the layout.
	if m.paneCount() > 1 {
		hints = append(hints,
			keyHint{key: "^B z", label: "zoom"},
			keyHint{key: "^B o", label: "next pane"},
		)
	}
	hints = append(hints,
		keyHint{key: "^B |", label: "split right"},
		keyHint{key: "^B -", label: "split down"},
	)
	if m.paneCount() > 1 {
		hints = append(hints, keyHint{key: "^B x", label: "close pane"})
	}
	if len(m.sessionNames()) > 1 {
		hints = append(hints, keyHint{key: "^B n/p", label: "next/prev session"})
	}
	return append(hints,
		keyHint{key: "^B c", label: "command"},
		keyHint{key: "^B ?", label: "help", keep: true},
		keyHint{key: "esc", label: "cancel", keep: true},
	)
}

// prefixMenuLine renders the Ctrl+B menu over the footer, with its click spans.
func (m model) prefixMenuLine(width int) (string, []hintSpan) {
	lead := warnChipStyle.Render(" ^B ") + " "
	leadW := lipgloss.Width(lead)
	bar, spans := renderHintBar(m.ctrlBHints(), max(0, width-leadW))
	for i := range spans {
		spans[i].start += leadW
		spans[i].end += leadW
	}
	return fitToWidth(lead+bar, width), spans
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
