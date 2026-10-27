package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/han/runic/internal/protocol"
)

func paneInfo(session, pane string, a protocol.Attention, note string, age time.Duration) protocol.TerminalInfo {
	return protocol.TerminalInfo{
		Session: session, Pane: pane, Attention: a, Note: note,
		Since: time.Now().Add(-age),
	}
}

// An agent that starts needing you rings once: on the transition, not on every
// poll that still finds it blocked.
func TestAlarmRingsOncePerAlert(t *testing.T) {
	var m model
	waiting := []protocol.TerminalInfo{
		paneInfo("agent-1", "p1", protocol.AttentionWaiting, "which schema?", time.Minute),
	}

	(&m).updateAlarm(waiting)
	if !m.alarm.bell {
		t.Fatal("a newly waiting pane should ring")
	}
	if got := m.alarm.alert.text(); !strings.Contains(got, "agent-1") || !strings.Contains(got, "which schema?") {
		t.Errorf("alert = %q, want it to name the session and what it asked", got)
	}

	// The frame carrying the bell has been rendered; Update retires it.
	(&m).clearBell()
	(&m).updateAlarm(waiting)
	if m.alarm.bell {
		t.Error("the same pane, still waiting, must not ring again")
	}
	if m.alarm.alert.empty() {
		t.Error("the alert should stay up while the agent is still blocked")
	}
}

// Attending to the pane clears the alert, and the *same* pane blocking again
// later is news again.
func TestAlarmReRingsAfterResolution(t *testing.T) {
	var m model
	blocked := []protocol.TerminalInfo{
		paneInfo("agent-1", "p1", protocol.AttentionWaiting, "which schema?", time.Minute),
	}
	(&m).updateAlarm(blocked)
	(&m).clearBell()

	// Attached to: waiting is resolved outright.
	(&m).updateAlarm([]protocol.TerminalInfo{
		paneInfo("agent-1", "p1", protocol.AttentionIdle, "", 0),
	})
	if !m.alarm.alert.empty() {
		t.Error("a resolved alert should leave the status bar")
	}
	if m.alarm.bell {
		t.Error("resolving an alert is not itself an alarm")
	}

	(&m).updateAlarm(blocked)
	if !m.alarm.bell {
		t.Error("the same agent blocking again should ring again")
	}
}

// An acknowledged error stops alarming: you know about it, and re-ringing every
// poll would be the dashboard nagging about something you have seen.
func TestAlarmSilentForAcknowledgedError(t *testing.T) {
	var m model
	(&m).updateAlarm([]protocol.TerminalInfo{
		paneInfo("broken", "p1", protocol.AttentionError, "tests failed", time.Minute),
	})
	if !m.alarm.bell {
		t.Fatal("a fresh error should ring")
	}
	(&m).clearBell()

	seen := paneInfo("broken", "p1", protocol.AttentionError, "tests failed", time.Minute)
	seen.Seen = true
	(&m).updateAlarm([]protocol.TerminalInfo{seen})
	if m.alarm.bell {
		t.Error("an acknowledged error must not ring")
	}
	if !m.alarm.alert.empty() {
		t.Error("an acknowledged error should drop off the alert too")
	}
}

// Working and idle panes are not alarms — only the states that need a human.
func TestAlarmIgnoresBusyAndQuietPanes(t *testing.T) {
	var m model
	(&m).updateAlarm([]protocol.TerminalInfo{
		paneInfo("a", "p1", protocol.AttentionWorking, "", time.Minute),
		paneInfo("b", "p1", protocol.AttentionIdle, "", time.Minute),
		paneInfo("c", "p1", protocol.AttentionNone, "", 0),
	})
	if m.alarm.bell || !m.alarm.alert.empty() {
		t.Error("an agent that needs nothing should not alarm")
	}
}

// Two agents blocking at once are two alerts, and the one that has waited
// longest is the one named — the same order the Agents list uses.
func TestAlarmNamesLongestWaiting(t *testing.T) {
	var m model
	(&m).updateAlarm([]protocol.TerminalInfo{
		paneInfo("recent", "p1", protocol.AttentionWaiting, "", 10*time.Second),
		paneInfo("patient", "p1", protocol.AttentionWaiting, "", 14*time.Minute),
	})
	if got := m.alarm.alert.text(); !strings.Contains(got, "patient") {
		t.Errorf("alert = %q, want the agent kept waiting longest", got)
	}
}

// Two agents in one session are two panes, so resolving one keeps the other's
// alert: the alarm keys on panes, not sessions.
func TestAlarmTracksPanesNotSessions(t *testing.T) {
	var m model
	both := []protocol.TerminalInfo{
		paneInfo("agent", "p1", protocol.AttentionWaiting, "first", 2*time.Minute),
		paneInfo("agent", "p2", protocol.AttentionError, "second", time.Minute),
	}
	(&m).updateAlarm(both)
	(&m).clearBell()
	if len(m.alarm.ringing) != 2 {
		t.Fatalf("tracking %d panes, want 2", len(m.alarm.ringing))
	}

	// p1 attended to; p2 still broken, and still the thing to say.
	(&m).updateAlarm([]protocol.TerminalInfo{
		paneInfo("agent", "p1", protocol.AttentionIdle, "", 0),
		both[1],
	})
	if len(m.alarm.ringing) != 1 {
		t.Errorf("tracking %d panes after one resolved, want 1", len(m.alarm.ringing))
	}
}

// The alert names the pane only when it has to, and always says *something*
// even with no note to quote.
func TestAlarmAlertText(t *testing.T) {
	cases := []struct {
		alert alarmAlert
		want  string
	}{
		{alarmAlert{session: "a1", note: "which schema?"}, "a1: which schema?"},
		{alarmAlert{session: "a1", pane: "p2", note: "stuck"}, "a1 · p2: stuck"},
		{alarmAlert{session: "a1", state: protocol.AttentionError}, "a1 failed"},
		{alarmAlert{session: "a1", state: protocol.AttentionWaiting}, "a1 needs you"},
	}
	for _, c := range cases {
		if got := c.alert.text(); got != c.want {
			t.Errorf("text() = %q, want %q", got, c.want)
		}
	}
}

// The bell rides inside the frame, which is the whole reason it is safe. It must
// lead the output and cost no display columns — a BEL that widened a line would
// push the layout a cell out on every alarm.
func TestBellLeadsTheFrameWithoutDisturbingLayout(t *testing.T) {
	m := tabsSessionModel()
	m.refreshTreeRows()
	quiet := m.View()

	m.alarm.bell = true
	loud := m.View()

	if !strings.HasPrefix(loud, "\a") {
		t.Fatal("the bell must lead the frame")
	}
	if strings.TrimPrefix(loud, "\a") != quiet {
		t.Error("the bell must be the only difference in the frame")
	}
	for i, line := range strings.Split(loud, "\n") {
		if w := lipgloss.Width(line); w != m.width {
			t.Fatalf("line %d is %d columns, want %d: the bell moved the layout", i, w, m.width)
		}
	}
	// Exactly one, so a frame cannot ring twice.
	if n := strings.Count(loud, "\a"); n != 1 {
		t.Errorf("frame carries %d bells, want 1", n)
	}
}

// Same contract in the jobs view, which is a separate render path.
func TestBellLeadsTheJobsFrame(t *testing.T) {
	m := tabsModel()
	m.alarm.bell = true
	out := m.View()
	if !strings.HasPrefix(out, "\a") {
		t.Fatal("the jobs view must carry the bell too")
	}
	for i, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w != m.width {
			t.Fatalf("line %d is %d columns, want %d", i, w, m.width)
		}
	}
}

// A live alert reaches both status bars, because an agent blocking while you
// work in another session is the case the alarm exists for.
func TestAlertShowsOnStatusBars(t *testing.T) {
	m := tabsSessionModel()
	m.width = 200
	m.refreshTreeRows()
	m.alarm.alert = alarmAlert{session: "agent-1", note: "which schema?", state: protocol.AttentionWaiting}

	if got := stripAnsi(m.renderFooter()); !strings.Contains(got, "which schema?") {
		t.Errorf("session status bar = %q, want the alert", got)
	}
	if got := stripAnsi(m.jobsStatusBar(200)); !strings.Contains(got, "which schema?") {
		t.Errorf("jobs status bar = %q, want the alert", got)
	}

	m.alarm.alert = alarmAlert{}
	if got := stripAnsi(m.jobsStatusBar(200)); strings.Contains(got, "which schema?") {
		t.Errorf("a cleared alert should leave the status bar: %q", got)
	}
}
