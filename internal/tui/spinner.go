package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/han/runic/internal/protocol"
)

// A working agent spins. Motion is the one thing a static screen cannot fake:
// a row that moves is visibly alive, where a "RUN" badge only claims to be.
// It also distinguishes "working" from every other state without another
// geometric glyph, which is what the icon vocabulary was trying to get away
// from.

// spinnerFrames is a braille cycle — eight frames, one column wide, no
// double-width surprises, and present in every font that has braille at all.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧"}

// spinnerInterval is fast enough to read as motion and slow enough to be
// cheap: eight frames a second, and only while something is actually working.
const spinnerInterval = 125 * time.Millisecond

type spinnerTickMsg struct{}

// spinnerTickCmd schedules the next frame.
func spinnerTickCmd() tea.Cmd {
	return tea.Tick(spinnerInterval, func(time.Time) tea.Msg { return spinnerTickMsg{} })
}

// spinner is the current frame.
func (m model) spinner() string {
	return spinnerFrames[m.spinnerFrame%len(spinnerFrames)]
}

// anyWorking reports whether any pane is working, which is the only condition
// under which the spinner ticks. An idle dashboard redraws on its poll and
// nothing else — a timer running at 8Hz over a quiet screen would be pure
// wakeups, and this TUI sits in the background for hours at a time.
func (m model) anyWorking() bool {
	for _, a := range m.agents {
		if a.Attention == protocol.AttentionWorking {
			return true
		}
	}
	return false
}

// syncSpinner starts or stops the frame timer to match whether anything is
// working, returning a command when it needs to start. spinnerOn guards against
// stacking a second timer on every poll.
func (m *model) syncSpinner() tea.Cmd {
	working := m.anyWorking()
	if working && !m.spinnerOn {
		m.spinnerOn = true
		return spinnerTickCmd()
	}
	if !working {
		m.spinnerOn = false
	}
	return nil
}
