package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// `q` has to mean "quit" from the sidebar too. It used to be swallowed there:
// handleTreeKey reported every key as handled, so the one key that reliably
// means "I am done" did nothing in the pane you are most often sitting in.
func TestQuitFromSidebar(t *testing.T) {
	quits := func(t *testing.T, m model, where string) {
		t.Helper()
		_, cmd, handled := m.handleTreeKey(keyRune('q'))
		if !handled {
			t.Fatalf("%s: the sidebar should handle q", where)
		}
		if cmd == nil {
			t.Fatalf("%s: q produced no command, want tea.Quit", where)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("%s: q did not quit", where)
		}
	}

	// From the management screen's sidebar.
	m := sideTabsModel()
	m.jobs.tree.focus = true
	quits(t, m, "management sidebar")

	// And from the sidebar beside an open session, where keys are not going to
	// the shell anyway because the sidebar holds focus.
	m = tabsSessionModel()
	m.jobs.tree.show = true
	m.jobs.tree.focus = true
	m.refreshTreeRows()
	quits(t, m, "session sidebar")
}

// The action menu is a modal, where `q` dismisses rather than quits — closing
// what you just opened is the expected escape, and quitting the app out from
// under an open menu would be a nasty surprise.
func TestQuitDoesNotEscapeTheActionMenu(t *testing.T) {
	m := sideTabsModel()
	m.jobs.tree.focus = true
	nm, _, _ := m.openTreeMenu()
	m = nm.(model)
	if !m.jobs.tree.menu.active {
		t.Fatal("expected the action menu to open")
	}
	got, cmd, handled := m.handleTreeKey(keyRune('q'))
	if !handled {
		t.Fatal("the menu should handle q")
	}
	if cmd != nil {
		t.Error("q in the menu should dismiss it, not quit the app")
	}
	if got.(model).jobs.tree.menu.active {
		t.Error("q should have closed the menu")
	}
}
