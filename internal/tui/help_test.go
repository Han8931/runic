package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func helpModel(w, h int) model {
	lipgloss.SetColorProfile(termenv.Ascii)
	m := tabsModel()
	m.width, m.height = w, h
	m.refreshJobsRows()
	m.jobs.helping = true
	return m
}

func helpText(m model) string {
	return stripAnsi(strings.Join(m.helpLines(m.jobsLayout().paneH, m.width-2), "\n"))
}

func pressHelp(t *testing.T, m model, key tea.KeyMsg) model {
	t.Helper()
	got, _ := m.handleJobsKey(key)
	return got.(model)
}

// The help list is far longer than a terminal, and centerBox silently emitted
// only the first bodyH lines — so at 80x24 everything past the middle was
// unreachable, with nothing on screen admitting it. Scrolling has to reach the
// end.
func TestHelpScrollsToTheEnd(t *testing.T) {
	m := helpModel(80, 24)
	top := helpText(m)

	for i := 0; i < 80; i++ {
		m = pressHelp(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	}
	end := helpText(m)
	if top == end {
		t.Fatal("scrolling changed nothing")
	}
	// The last section used to be below the fold at this size.
	if !strings.Contains(end, "Reading the screen") {
		t.Errorf("the final section should be reachable: %q", end)
	}
	// And the page says where it is, so a clipped page is visibly clipped.
	if !strings.Contains(top, "top") {
		t.Errorf("the help footer should report the position: %q", top)
	}
	if !strings.Contains(end, "end") {
		t.Errorf("scrolled to the bottom, the footer should say so: %q", end)
	}
}

// G and gg reach both ends; the offset is clamped to the real content length
// rather than scrolling into blank space.
func TestHelpJumpsAndClamps(t *testing.T) {
	m := helpModel(80, 24)
	m = pressHelp(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	end := helpText(m)
	if !strings.Contains(end, "end") {
		t.Errorf("G should jump to the bottom: %q", end)
	}
	// Pressing on past the end must not empty the page.
	for i := 0; i < 20; i++ {
		m = pressHelp(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	}
	if got := helpText(m); !strings.Contains(got, "Reading the screen") {
		t.Errorf("over-scrolling emptied the page: %q", got)
	}

	m = pressHelp(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if got := helpText(m); !strings.Contains(got, "top") {
		t.Errorf("g should return to the top: %q", got)
	}
}

// Any key used to dismiss the overlay, so reaching for a scroll key threw the
// page away. Only the deliberate dismissals close it.
func TestHelpClosesOnlyDeliberately(t *testing.T) {
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyEsc},
		{Type: tea.KeyRunes, Runes: []rune{'q'}},
		{Type: tea.KeyRunes, Runes: []rune{'?'}},
	} {
		m := pressHelp(t, helpModel(80, 24), key)
		if m.jobs.helping {
			t.Errorf("%v should close the help", key)
		}
	}
	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'j'}},
		{Type: tea.KeyRunes, Runes: []rune{'x'}},
		{Type: tea.KeyCtrlD},
		{Type: tea.KeyDown},
	} {
		m := pressHelp(t, helpModel(80, 24), key)
		if !m.jobs.helping {
			t.Errorf("%v should not dismiss the help mid-read", key)
		}
	}
}

// The page opens on what applies right now. The full reference is long enough
// that the top of it has to be the part you came looking for.
func TestHelpLeadsWithTheCurrentContext(t *testing.T) {
	m := helpModel(100, 30)
	got := helpText(m)
	if !strings.Contains(got, "Right now") {
		t.Errorf("help should open with a contextual section: %q", got)
	}
	// The contextual block is drawn from the same hints the footer shows, so it
	// cannot drift from what the keys actually do.
	if !strings.Contains(got, "Right now") || !strings.Contains(got, "Navigation") {
		t.Errorf("expected both the contextual and the full sections: %q", got)
	}
	if strings.Index(got, "Right now") > strings.Index(got, "Navigation") {
		t.Error("the contextual section should come first")
	}
}

// Whatever the terminal size, the overlay fits its pane exactly: an overlay
// that overflows corrupts the frame around it.
func TestHelpFitsEveryPane(t *testing.T) {
	for _, wh := range [][2]int{{80, 24}, {100, 30}, {140, 40}, {60, 12}} {
		m := helpModel(wh[0], wh[1])
		body := m.jobsLayout().paneH
		lines := m.helpLines(body, m.width-2)
		if len(lines) != body {
			t.Errorf("%dx%d: help rendered %d lines into a %d-row pane", wh[0], wh[1], len(lines), body)
		}
		for _, l := range lines {
			if w := lipgloss.Width(l); w > m.width-2 {
				t.Errorf("%dx%d: a help line is %d columns, pane inner is %d", wh[0], wh[1], w, m.width-2)
			}
		}
	}
}

// The wheel scrolls the page rather than moving the job cursor underneath it.
func TestHelpScrollsWithTheWheel(t *testing.T) {
	m := helpModel(80, 24)
	before := m.jobs.cursor
	got, _ := m.handleMouse(tea.MouseMsg{
		Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress, X: 10, Y: 5,
	})
	m = got.(model)
	if m.jobs.help.offset == 0 {
		t.Error("the wheel should scroll the help")
	}
	if m.jobs.cursor != before {
		t.Error("the wheel must not move the job cursor while help is open")
	}
}
