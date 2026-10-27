package tui

import (
	"fmt"

	"github.com/han/runic/internal/protocol"
)

// The attention alarm: an agent that starts needing you says so out loud, once,
// instead of waiting to be noticed on a tab or in a sidebar. Two signals, both
// driven off the same transition:
//
//   - a terminal bell, for when you are looking at something else entirely;
//   - a status-bar alert naming the session and what it asked, for when the
//     bell is disabled (or you missed it) and you want to know which agent.
//
// The bell is emitted as part of the rendered frame rather than written to the
// terminal on the side. That is what makes it safe: the frame is one write, made
// by the renderer itself, so a BEL inside it cannot land in the middle of an
// escape sequence. Writing "\a" to stdout from Update — the obvious approach —
// races the renderer's own writes, which is why this was left undone.

// alarmState tracks what has already been announced, so an alert alarms on the
// transition into needing you rather than on every poll that still finds it.
type alarmState struct {
	// ringing is the set of panes currently counted as alerting, keyed by
	// session\x00pane. A pane leaves the set when its alert resolves, which is
	// what lets the *same* pane alarm again next time it blocks.
	ringing map[string]bool
	// bell is set for exactly one frame: View emits the BEL and the next
	// message through Update clears it.
	bell bool
	// alert names the agent that most recently started needing you, and what it
	// said. Held until that pane's alert is resolved or acknowledged.
	alert alarmAlert
}

type alarmAlert struct {
	session string
	pane    string
	note    string
	state   protocol.Attention
}

func (a alarmAlert) empty() bool { return a.session == "" }

// text is the alert as it reads on the status bar: who needs you and why, with
// the note in their own words when there is one.
func (a alarmAlert) text() string {
	label := a.session
	if a.pane != "" {
		label = a.session + " · " + a.pane
	}
	switch {
	case a.note != "":
		return fmt.Sprintf("%s: %s", label, a.note)
	case a.state == protocol.AttentionError:
		return label + " failed"
	default:
		return label + " needs you"
	}
}

// updateAlarm folds a fresh pane poll into the alarm state, ringing for panes
// that have newly become unresolved and forgetting the ones that no longer are.
//
// Panes are compared by identity, not by count: two agents blocking at once
// must alarm twice, and a pane that is still waiting from the last poll must
// not alarm again.
func (m *model) updateAlarm(panes []protocol.TerminalInfo) {
	stillRinging := make(map[string]bool, len(panes))
	var newest *protocol.TerminalInfo
	for i := range panes {
		p := &panes[i]
		if !protocol.Unresolved(p.Attention, p.Seen) {
			continue
		}
		key := p.Session + "\x00" + p.Pane
		stillRinging[key] = true
		if m.alarm.ringing[key] {
			continue // already announced; not news
		}
		// Newest by when the pane entered the state, so a poll that first sees
		// two blocked agents alerts about the one that has waited longest —
		// the same order the Agents list uses.
		if newest == nil || p.Since.Before(newest.Since) {
			newest = p
		}
	}
	m.alarm.ringing = stillRinging
	if newest != nil {
		m.alarm.bell = true
		m.alarm.alert = alarmAlert{
			session: newest.Session,
			pane:    newest.Pane,
			note:    newest.Note,
			state:   newest.Attention,
		}
	}
	// Drop a held alert once its pane stops needing you — by being attended to,
	// acknowledged, or closed. Leaving it up would be the dashboard lying in the
	// other direction.
	if !m.alarm.alert.empty() {
		key := m.alarm.alert.session + "\x00" + m.alarm.alert.pane
		if !stillRinging[key] {
			m.alarm.alert = alarmAlert{}
		}
	}
}

// bellPrefix returns the BEL to lead the next frame with, and is empty on every
// other frame. Pairing it with clearBell in Update is what makes the ring
// happen once.
func (m model) bellPrefix() string {
	if m.alarm.bell {
		return "\a"
	}
	return ""
}

// clearBell retires a bell that the last frame has by now carried. Called at
// the top of Update, so the flag survives exactly one render.
func (m *model) clearBell() { m.alarm.bell = false }
