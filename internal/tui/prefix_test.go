package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Pressing Ctrl+B says what Ctrl+B can do. A chord you have to already know is
// not a feature anyone discovers, and the help page is the wrong place to learn
// it — you need it at the moment your fingers are already on the prefix.
func TestPrefixMenuAppearsWhilePrefixIsLive(t *testing.T) {
	lipgloss.SetColorProfile(termenv.Ascii)
	m := tabsSessionModel()
	m.width = 120
	m.refreshTreeRows()

	quiet := stripAnsi(m.View())
	if strings.Contains(quiet, "detach") && strings.Contains(quiet, "cancel") {
		t.Fatal("the menu should not be on screen before the prefix is pressed")
	}

	nm, handled, _ := m.handleCtrlB(tea.KeyMsg{Type: tea.KeyCtrlB})
	if !handled || !nm.ctrlBPressed {
		t.Fatal("Ctrl+B should arm the prefix")
	}
	loud := stripAnsi(nm.View())
	for _, want := range []string{"^B", "detach", "help", "cancel"} {
		if !strings.Contains(loud, want) {
			t.Errorf("the prefix menu should list %q: %q", want, lastLine(loud))
		}
	}
	// It replaces the status bar rather than adding a row, so the frame keeps
	// its height.
	if a, b := len(strings.Split(quiet, "\n")), len(strings.Split(loud, "\n")); a != b {
		t.Errorf("the menu changed the frame height: %d -> %d", a, b)
	}
}

// The chords it lists are the ones that apply: nothing about panes when there
// is only one, nothing about sessions when there is only one.
func TestPrefixMenuListsOnlyWhatApplies(t *testing.T) {
	m := tabsSessionModel()
	single := m.ctrlBHints()
	for _, h := range single {
		if h.label == "zoom" || h.label == "close pane" {
			t.Errorf("a single pane should not advertise %q", h.label)
		}
	}

	two := zoomModel(t)
	var sawZoom bool
	for _, h := range two.ctrlBHints() {
		if h.label == "zoom" {
			sawZoom = true
		}
	}
	if !sawZoom {
		t.Error("with two panes, zoom should be offered")
	}
}

// The hint bar drops from the end, so the hints that matter most when you are
// lost — help, quit, cancel — are conventionally the first to go. They are
// pinned instead.
func TestProtectedHintsSurviveANarrowBar(t *testing.T) {
	lipgloss.SetColorProfile(termenv.Ascii)
	hints := []keyHint{
		{key: "a", label: "alpha"},
		{key: "b", label: "bravo"},
		{key: "c", label: "charlie"},
		{key: "?", label: "help", keep: true},
		{key: "q", label: "quit", keep: true},
	}
	for _, width := range []int{30, 40, 60, 120} {
		bar, spans := renderHintBar(hints, width)
		plain := stripAnsi(bar)
		if lipgloss.Width(bar) != width {
			t.Errorf("%d: bar rendered %d columns", width, lipgloss.Width(bar))
		}
		for _, want := range []string{"help", "quit"} {
			if !strings.Contains(plain, want) {
				t.Errorf("%d: dropped the protected hint %q: %q", width, want, plain)
			}
		}
		for _, sp := range spans {
			if sp.end > width {
				t.Errorf("%d: span for %q runs past the bar", width, sp.hint.label)
			}
		}
	}
	// With room for everything, order is still the order given.
	plain := stripAnsi(mustBar(hints, 200))
	if strings.Index(plain, "alpha") > strings.Index(plain, "help") {
		t.Error("protecting a hint must not reorder the bar when it all fits")
	}
}

func mustBar(hints []keyHint, w int) string {
	bar, _ := renderHintBar(hints, w)
	return bar
}

// Clicking a chord in the menu runs it, through the same path the keyboard uses.
func TestPrefixMenuIsClickable(t *testing.T) {
	lipgloss.SetColorProfile(termenv.Ascii)
	m := zoomModel(t)
	m.ctrlBPressed = true

	_, spans := m.prefixMenuLine(m.width)
	var zoom hintSpan
	for _, sp := range spans {
		if sp.hint.label == "zoom" {
			zoom = sp
		}
	}
	if zoom.end == 0 {
		t.Fatal("expected a zoom target in the menu")
	}
	got, _ := m.handleMouse(leftClick(zoom.start+1, m.height-1))
	if !got.(model).paneZoom {
		t.Error("clicking zoom in the prefix menu should zoom")
	}
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	return lines[len(lines)-1]
}
