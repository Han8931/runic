package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Mouse affordances shared by both screens: double-click to open, and a
// clickable hint bar.
//
// The hint bar is the whole trick. It already lists exactly the keys that apply
// right now, so making those entries clickable turns it into a context toolbar
// — a complete mouse path through the app with no new UI to design, nothing to
// memorize, and no second definition of what each verb does: a click replays
// the keystroke the bar is advertising, through the same handler the keyboard
// uses. A hint cannot drift from its button.

// doubleClickWindow is how long after a click a second one on the same target
// still reads as a double click.
const doubleClickWindow = 400 * time.Millisecond

// clickState remembers the last click so the next one can be recognized as a
// double. The target is the row's identity rather than its screen position, so
// a list that reorders under the cursor between polls cannot turn two clicks on
// different things into one activation.
type clickState struct {
	target string
	at     time.Time
}

// doubleClick records a click on target and reports whether it completes a
// double click. The pair is consumed, so three clicks are one double and one
// single rather than two doubles.
func (m *model) doubleClick(target string) bool {
	now := time.Now()
	double := m.lastClick.target == target && now.Sub(m.lastClick.at) <= doubleClickWindow
	if double {
		m.lastClick = clickState{}
		return true
	}
	m.lastClick = clickState{target: target, at: now}
	return false
}

// treeRowTarget identifies a sidebar row for double-click purposes.
func treeRowTarget(r treeRow) string {
	return fmt.Sprintf("tree|%d|%s|%s|%s|%d", r.kind, r.group, r.session, r.pane, r.job.ID)
}

// hintPress maps a hint's key cap back to the keystroke it advertises: the key
// to send, and whether it needs the Ctrl+B prefix first.
//
// Only unambiguous caps are clickable. `j/k` names two keys and `gg/G` a chord,
// so neither becomes a button — a hint that did something arbitrary when
// clicked would be worse than one that does nothing.
func hintPress(cap string) (key string, viaCtrlB bool, ok bool) {
	if rest, found := strings.CutPrefix(cap, "^B "); found {
		if len([]rune(rest)) == 1 {
			return rest, true, true
		}
		return "", false, false
	}
	switch cap {
	case "⏎", "enter":
		return "enter", false, true
	case "tab":
		return "tab", false, true
	case "esc":
		return "esc", false, true
	case "space":
		return " ", false, true
	}
	if len([]rune(cap)) == 1 {
		return cap, false, true
	}
	return "", false, false
}

// hintKeyMsg builds the key message a hint click replays.
func hintKeyMsg(key string) (tea.KeyMsg, bool) {
	switch key {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}, true
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}, true
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}, true
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace}, true
	}
	if r := []rune(key); len(r) == 1 {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: r}, true
	}
	return tea.KeyMsg{}, false
}

// hintAt resolves a click at column x on a hint bar to the hint it landed on.
func hintAt(spans []hintSpan, x int) (keyHint, bool) {
	for _, sp := range spans {
		if x >= sp.start && x < sp.end {
			return sp.hint, true
		}
	}
	return keyHint{}, false
}

// pressHint replays the keystroke a hint advertises. Chord hints go through the
// Ctrl+B handler exactly as the keyboard would; plain hints go to the screen's
// own key handler. Nothing here knows what any verb *does*, which is the point.
func (m model) pressHint(h keyHint) (tea.Model, tea.Cmd, bool) {
	key, viaCtrlB, ok := hintPress(h.key)
	if !ok {
		return m, nil, false
	}
	msg, ok := hintKeyMsg(key)
	if !ok {
		return m, nil, false
	}
	if viaCtrlB {
		m.ctrlBPressed = true
		nm, handled, cmd := m.handleCtrlB(msg)
		if !handled {
			// The chord was not recognized, and handleCtrlB has already forwarded
			// the buffered Ctrl+B to the shell. Do not also deliver the key.
			return nm, cmd, true
		}
		return nm, cmd, true
	}
	if m.viewMode == viewJobs || m.jobs.tree.focus {
		nm, cmd := m.handleJobsKey(msg)
		return nm, cmd, true
	}
	// In a session with the terminal focused, a plain key belongs to the shell;
	// clicking a hint must not type into it.
	return m, nil, false
}
