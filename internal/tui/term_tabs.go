package tui

import (
	"sort"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/han/runic/internal/protocol"
)

// Session-tab navigation and the per-session state the tabs display. The point
// of the tabs is not just navigation: each carries its session's attention
// glyph, so an agent that starts waiting on you is visible *while you are
// working in another session*. Without them, noticing meant detaching to the
// management screen.
//
// The strip itself is rendered by sessionStrip (session_navigation.go), which
// the jobs view shares — the tabs scroll to keep the active session visible
// rather than being all-or-nothing.

// openSessionTabs lists the sessions already holding a live pane layout — the
// ones switching to costs nothing. The tabs themselves span the daemon's whole
// catalog (sessionNames); this is the fallback for before the first catalog
// poll lands, when the open layouts are all this client knows about.
//
// The order is by name rather than by when each was opened, matching
// sessionNames so the two agree whenever both are non-empty.
func (m model) openSessionTabs() []string {
	names := make([]string, 0, len(m.layouts))
	for name, root := range m.layouts {
		if root != nil {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// termTabSpan is where one tab landed in the rendered strip, in columns from
// the left edge of the region, so a click can be resolved back to its session
// (handleSessionMouse).
type termTabSpan struct {
	session    string
	start, end int // [start, end)
}

// cycleSessionTab switches to the next/previous open session, wrapping around.
// A single open session has nowhere to go, so it is a no-op.
func (m model) cycleSessionTab(delta int) (model, tea.Cmd) {
	tabs := m.sessionNames()
	if len(tabs) < 2 {
		return m, nil
	}
	at := 0
	for i, name := range tabs {
		if name == m.activeSession {
			at = i
			break
		}
	}
	n := len(tabs)
	return m.activateSession(tabs[((at+delta)%n+n)%n])
}

// selectSessionTab switches to the 1-based nth open session, if it exists.
func (m model) selectSessionTab(n int) (model, tea.Cmd) {
	tabs := m.sessionNames()
	if n < 1 || n > len(tabs) {
		return m, nil
	}
	if tabs[n-1] == m.activeSession {
		return m, nil
	}
	return m.activateSession(tabs[n-1])
}

// sessionAttention is the rolled-up state of a session from the last poll, or
// AttentionNone when it has reported nothing. An acknowledged error reports as
// none here: the tab should not wear an alert glyph for something already seen.
func (m model) sessionAttention(name string) protocol.Attention {
	s, ok := m.sessionInfo(name)
	if !ok {
		return protocol.AttentionNone
	}
	switch {
	case protocol.Unresolved(s.Attention, s.AttentionSeen):
		return s.Attention
	case s.Attention == protocol.AttentionWorking:
		return protocol.AttentionWorking
	default:
		// Idle, nothing reported, or an error already acknowledged: no glyph,
		// so the strip only ever marks live news.
		return protocol.AttentionNone
	}
}
