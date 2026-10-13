package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// The split view's status bar, in the same gorae style as the management
// screen's: mode chip, LABEL value pairs, and key caps on the right.

func (m model) renderFooter() string {
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

	right := ""
	if m.gitBranch != "" {
		right += statusPair("BRANCH", val("⊕ "+m.gitBranch))
	}
	right += statusPair("JOBS", val(fmt.Sprint(m.jobCount()))) +
		statusPair("SLOTS", val(fmt.Sprint(m.maxSlots))) + " " +
		statusKeys([]keyHint{{"^B d", "detach"}, {"^B |/-", "split"}})
	return renderStatusBar(m.width, left, right)
}

// statusKeys renders key caps for the status bar's right edge.
func statusKeys(hints []keyHint) string {
	var parts []string
	for _, h := range hints {
		parts = append(parts, keyCapStyle.Render(" "+h.key+" ")+statusValueStyle.Foreground(cFgDim).Render(" "+h.label+" "))
	}
	return strings.Join(parts, statusBarStyle.Render(" "))
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
