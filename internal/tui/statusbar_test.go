package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// The footer used to drop its *entire* right side the moment it did not fit,
// which took the navigation keys with it — so at 80 columns nothing on screen
// said that detach existed, and the clickable toolbar had no targets. The way
// out of where you are has to survive every width.
func TestFooterKeepsEscapeHatchesAtEveryWidth(t *testing.T) {
	lipgloss.SetColorProfile(termenv.Ascii)
	for _, width := range []int{60, 80, 100, 120, 140, 200} {
		m := tabsSessionModel()
		m.width = width
		m.gitBranch = "feat/ui_update"
		m.refreshTreeRows()

		line, spans := m.footerSpans()
		if got := lipgloss.Width(line); got != width {
			t.Errorf("%d: footer is %d columns", width, got)
		}
		plain := stripAnsi(line)
		for _, want := range []string{"detach", "jobs", "help"} {
			if !strings.Contains(plain, want) {
				t.Errorf("%d: footer dropped %q: %q", width, want, plain)
			}
		}
		// And they are clickable, not just printed.
		labels := map[string]bool{}
		for _, sp := range spans {
			labels[sp.hint.label] = true
			if sp.end > width {
				t.Errorf("%d: span for %q runs past the bar", width, sp.hint.label)
			}
		}
		for _, want := range []string{"detach", "jobs", "help"} {
			if !labels[want] {
				t.Errorf("%d: %q is not clickable", width, want)
			}
		}
	}
}

// The mode chip says where you are and the alarm says who needs you; neither is
// ever the thing that gets given up.
func TestFooterKeepsModeAndAlarmWhenCramped(t *testing.T) {
	lipgloss.SetColorProfile(termenv.Ascii)
	m := tabsSessionModel()
	m.width = 70
	m.gitBranch = "a-very-long-branch-name-that-will-not-fit"
	m.alarm.alert = alarmAlert{session: "agent-1", note: "which schema?"}
	m.refreshTreeRows()

	plain := stripAnsi(mustLine(t, m))
	if !strings.Contains(plain, "INSERT") {
		t.Errorf("the mode chip must survive: %q", plain)
	}
	if !strings.Contains(plain, "agent-1") {
		t.Errorf("the alarm must survive: %q", plain)
	}
	// The readouts are what gets dropped instead.
	if strings.Contains(plain, "SLOTS") {
		t.Errorf("a slot count should go before anything that matters: %q", plain)
	}
}

// Readouts come back as the terminal gets wider, so none of them is dead
// weight — they are deferred, not removed.
func TestFooterRestoresReadoutsWhenThereIsRoom(t *testing.T) {
	lipgloss.SetColorProfile(termenv.Ascii)
	narrow := tabsSessionModel()
	narrow.width = 80
	narrow.refreshTreeRows()
	wide := tabsSessionModel()
	wide.width = 220
	wide.refreshTreeRows()

	if n, w := stripAnsi(mustLine(t, narrow)), stripAnsi(mustLine(t, wide)); strings.Contains(n, "SLOTS") || !strings.Contains(w, "SLOTS") {
		t.Errorf("SLOTS should appear only when there is room\nnarrow: %q\nwide:   %q", n, w)
	}
}

func mustLine(t *testing.T, m model) string {
	t.Helper()
	line, _ := m.footerSpans()
	return line
}

// fitBar's contract, independent of what is on the bar: exact width, protected
// segments kept, most-expendable dropped first.
func TestFitBarShedsByPriority(t *testing.T) {
	lipgloss.SetColorProfile(termenv.Ascii)
	left := []barSegment{{text: "MODE"}, {text: "-session-", drop: 2}}
	right := []barSegment{{text: "-counts-", drop: 3}, {text: "KEYS"}}

	// Everything fits.
	line, _ := fitBar(40, left, right)
	plain := stripAnsi(line)
	for _, want := range []string{"MODE", "-session-", "-counts-", "KEYS"} {
		if !strings.Contains(plain, want) {
			t.Errorf("at 40 columns %q should be present: %q", want, plain)
		}
	}
	if lipgloss.Width(line) != 40 {
		t.Errorf("width = %d, want 40", lipgloss.Width(line))
	}

	// Too narrow: the counts go first, then the session, never MODE or KEYS.
	line, _ = fitBar(17, left, right)
	plain = stripAnsi(line)
	if strings.Contains(plain, "-counts-") {
		t.Errorf("the most expendable segment should have gone: %q", plain)
	}
	if !strings.Contains(plain, "MODE") || !strings.Contains(plain, "KEYS") {
		t.Errorf("protected segments must survive: %q", plain)
	}
	if lipgloss.Width(line) != 17 {
		t.Errorf("width = %d, want 17", lipgloss.Width(line))
	}

	// Narrower than the protected segments themselves: truncate, do not panic.
	for _, w := range []int{0, 1, 5, 8} {
		line, _ = fitBar(w, left, right)
		if lipgloss.Width(line) != w {
			t.Errorf("at %d columns the bar rendered %d", w, lipgloss.Width(line))
		}
	}
}
