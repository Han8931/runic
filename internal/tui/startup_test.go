package tui

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Keep real rendering and input decoding, but omit daemon polling and timers.
type startupProbe struct{ model }

func (m startupProbe) Init() tea.Cmd { return nil }
func (m startupProbe) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.model.Update(msg)
	m.model = next.(model)
	if _, ok := msg.(tea.MouseMsg); ok {
		return m, tea.Quit
	}
	return m, cmd
}

func TestStartupEnablesMouseAndAcceptsTabClick(t *testing.T) {
	m := agentsViewModel()
	// Match startup: openJobsView marks mouseOn, but its command is discarded.
	m, _ = m.openJobsView()
	line := strings.Split(stripAnsi(m.View()), "\n")[mgmtTabStripLine]
	idx := strings.LastIndex(line, "Agents")
	if idx < 0 {
		t.Fatal("missing Agents tab")
	}
	x := lipgloss.Width(line[:idx])
	// SGR mouse coordinates are one-based; click the actual rendered label.
	input := strings.NewReader(fmt.Sprintf("\x1b[<0;%d;%dM", x+1, mgmtTabStripLine+1))
	var output bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	p := newTUIProgram(startupProbe{m}, tea.WithInput(input), tea.WithOutput(&output), tea.WithContext(ctx), tea.WithoutSignalHandler())
	final, err := p.Run()
	if err != nil {
		t.Fatal(err)
	}
	if final.(startupProbe).jobs.main != mainAgents {
		t.Fatal("terminal mouse input did not switch to Agents")
	}
	for _, sequence := range []string{"\x1b[?1002h", "\x1b[?1006h"} {
		if !strings.Contains(output.String(), sequence) {
			t.Errorf("startup did not enable mouse mode %q", sequence)
		}
	}
}
