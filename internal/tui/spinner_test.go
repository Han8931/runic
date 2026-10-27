package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/han/runic/internal/protocol"
)

// The spinner runs only while something is working. This TUI sits open in the
// background for hours; a timer ticking at 8Hz over a quiet screen would be
// pure wakeups for an animation nobody is watching.
func TestSpinnerRunsOnlyWhileWorking(t *testing.T) {
	var m model
	if cmd := (&m).syncSpinner(); cmd != nil || m.spinnerOn {
		t.Error("nothing is working, so nothing should be ticking")
	}

	m.agents = []protocol.TerminalInfo{
		{Session: "a", Pane: "p1", Attention: protocol.AttentionWorking, Since: time.Now()},
	}
	cmd := (&m).syncSpinner()
	if cmd == nil || !m.spinnerOn {
		t.Fatal("a working agent should start the spinner")
	}
	// A second poll while it is already running must not stack another timer.
	if again := (&m).syncSpinner(); again != nil {
		t.Error("the spinner should not be started twice")
	}

	// The agent finishes; the next poll stops it.
	m.agents[0].Attention = protocol.AttentionIdle
	if cmd := (&m).syncSpinner(); cmd != nil || m.spinnerOn {
		t.Error("the spinner should stop once nothing is working")
	}
}

// A tick advances the frame and reschedules itself — but only while there is
// still something to animate.
func TestSpinnerTickStopsWhenWorkEnds(t *testing.T) {
	m := model{spinnerOn: true}
	m.agents = []protocol.TerminalInfo{
		{Session: "a", Pane: "p1", Attention: protocol.AttentionWorking},
	}
	first := m.spinner()

	got, cmd := m.Update(spinnerTickMsg{})
	m = got.(model)
	if cmd == nil {
		t.Error("a tick should schedule the next frame while work continues")
	}
	if m.spinner() == first {
		t.Error("the frame should have advanced")
	}

	m.agents[0].Attention = protocol.AttentionIdle
	got, cmd = m.Update(spinnerTickMsg{})
	if cmd != nil {
		t.Error("the last tick should not reschedule")
	}
	if got.(model).spinnerOn {
		t.Error("spinnerOn should be cleared when the animation stops")
	}
}

// Frames cycle rather than running off the end of the slice.
func TestSpinnerFramesWrap(t *testing.T) {
	m := model{}
	seen := map[string]bool{}
	for i := 0; i < len(spinnerFrames)*3; i++ {
		m.spinnerFrame = i
		f := m.spinner()
		if f == "" {
			t.Fatalf("frame %d is empty", i)
		}
		seen[f] = true
	}
	if len(seen) != len(spinnerFrames) {
		t.Errorf("saw %d distinct frames, want %d", len(seen), len(spinnerFrames))
	}
}

// A working row shows the live frame; the retired glyphs stay retired.
func TestWorkingRowAnimates(t *testing.T) {
	m := sideTabsModel()
	m.jobs.tree.tab = sideAgents
	m.refreshTreeRows()

	var working treeRow
	for _, r := range m.jobs.tree.rows {
		if r.attention == protocol.AttentionWorking {
			working = r
		}
	}
	if working.session == "" {
		t.Fatal("expected a working agent in the fixture")
	}

	m.spinnerFrame = 0
	a := stripAnsi(m.renderTreeRow(working, false, treePaneWidth))
	m.spinnerFrame = 4
	b := stripAnsi(m.renderTreeRow(working, false, treePaneWidth))
	if a == b {
		t.Error("a working row should change between spinner frames")
	}
	for _, glyph := range []string{"◐", "●", "×", "◆"} {
		if strings.Contains(a, glyph) {
			t.Errorf("retired glyph %q is back: %q", glyph, a)
		}
	}
}

// The badge vocabulary, including what acknowledgement does to it.
func TestAttentionBadges(t *testing.T) {
	cases := []struct {
		a     protocol.Attention
		seen  bool
		badge string
		loud  bool
	}{
		{protocol.AttentionWaiting, false, "WAIT", true},
		{protocol.AttentionWaiting, true, "WAIT", false},
		{protocol.AttentionError, false, "ERR", true},
		{protocol.AttentionError, true, "ERR", false},
		{protocol.AttentionIdle, false, "", false},
		{protocol.AttentionNone, false, "", false},
	}
	for _, c := range cases {
		badge, style, reported := attentionBadge(c.a, c.seen, "⠋")
		if badge != c.badge {
			t.Errorf("%v seen=%v: badge = %q, want %q", c.a, c.seen, badge, c.badge)
		}
		if reported != (c.badge != "") {
			t.Errorf("%v seen=%v: reported = %v", c.a, c.seen, reported)
		}
		// An acknowledged alert keeps its words and gives up its color.
		if c.badge != "" {
			quiet := style.GetForeground() == treeSummaryStyle.GetForeground()
			if c.loud && quiet {
				t.Errorf("%v seen=%v should keep its color", c.a, c.seen)
			}
			if !c.loud && !quiet {
				t.Errorf("%v seen=%v should have gone quiet", c.a, c.seen)
			}
		}
	}
	// Working borrows the caller's live frame rather than a static word.
	if badge, _, _ := attentionBadge(protocol.AttentionWorking, false, "⠹"); badge != "⠹" {
		t.Errorf("working badge = %q, want the spinner frame", badge)
	}
}

// A session tab has one column, so it gets a pip rather than a word — and the
// pip for a working session is the live frame.
func TestSessionPipIsOneColumn(t *testing.T) {
	m := sideTabsModel()
	m.spinnerFrame = 2
	for _, name := range []string{"build", "default"} {
		pip, _, _ := m.sessionPip(name)
		if len([]rune(pip)) != 1 {
			t.Errorf("pip for %q is %q, want one column", name, pip)
		}
	}
}

var _ tea.Cmd = spinnerTickCmd()
