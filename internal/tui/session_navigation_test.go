package tui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/han/runic/internal/protocol"
	"strings"
	"testing"
	"time"
)

func TestSessionFiltersComposeWithJobStatus(t *testing.T) {
	m := tabsModel()
	jobs := append(m.collectJobs(), protocol.JobInfo{ID: 20, Session: "other", State: protocol.StateRunning})
	m.nodes = buildTree([]string{"default"}, []protocol.SessionInfo{{Name: "default", Group: "default"}, {Name: "other", Group: "default"}}, jobs)
	m.jobs.allJobs = jobs
	m.selectJobsSession("other")
	if got := fmt.Sprint(rowIDs(m)); got != "[20]" {
		t.Fatalf("session rows: %s", got)
	}
	if m.tabCount(tabRunning) != 1 || m.tabCount(tabDone) != 0 {
		t.Fatal("status counts escaped session scope")
	}
	m.switchTab(tabDone)
	if len(m.jobs.rows) != 0 {
		t.Fatal("status filter escaped session scope")
	}
	m.selectJobsSession("")
	if len(m.jobs.rows) != 2 {
		t.Fatal("all sessions did not restore completed jobs")
	}
}

func TestManagementSessionTabMouseAndOrder(t *testing.T) {
	m := tabsModel()
	l := m.jobsLayout()
	_, spans := m.jobsSessionStrip(l.listW - 2)
	target := spans[1]
	nm, _ := m.handleMouse(tea.MouseMsg{X: l.listX + 1 + target.start, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = nm.(model)
	if m.jobs.session != "default" {
		t.Fatal("session tab did not select scope")
	}
	lines := strings.Split(m.View(), "\n")
	if !strings.Contains(stripAnsi(lines[0]), "default") || !strings.Contains(stripAnsi(lines[1]), "Running") {
		t.Fatal("session tabs must sit above job status tabs")
	}
}

func TestSessionStripKeepsActiveVisible(t *testing.T) {
	m := tabsSessionModel()
	names := []string{"alpha", "beta", "a-very-long-session-name-with-界", "omega"}
	for _, width := range []int{12, 20, 40, 80} {
		for _, active := range names {
			strip, spans := m.sessionStrip(width, names, active)
			if lipgloss.Width(strip) != width {
				t.Fatalf("width %d: got %d", width, lipgloss.Width(strip))
			}
			found := false
			for _, sp := range spans {
				if sp.session == active {
					found = true
				}
				if sp.end > width {
					t.Fatal("click span outside strip")
				}
			}
			if !found {
				t.Fatalf("active tab %q missing at width %d", active, width)
			}
		}
	}
}

func TestSessionSidebarKeyboardAndMouse(t *testing.T) {
	m := tabsSessionModel()
	m.jobs.tree.show = true
	m.refreshTreeRows()
	m.ctrlBPressed = true
	m, handled, _ := m.handleCtrlB(keyRune('s'))
	if !handled || !m.jobs.tree.focus {
		t.Fatal("prefix s did not focus sidebar")
	}
	nm, _ := m.Update(keyRune('n'))
	m = nm.(model)
	if !m.jobs.form.active || m.viewMode != viewSplit {
		t.Fatal("new session must open inline")
	}
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(model)
	if m.jobs.form.active {
		t.Fatal("escape did not close form")
	}
	target := -1
	for i, r := range m.jobs.tree.rows {
		if r.session == "agent-1" {
			target = i
			break
		}
	}
	if target < 0 {
		t.Fatal("session missing")
	}
	nm, _ = m.handleMouse(tea.MouseMsg{X: 5, Y: treeBodyTopLine + target, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = nm.(model)
	if m.activeSession != "agent-1" || m.jobs.tree.focus {
		t.Fatal("click did not switch and focus terminal")
	}
}

func TestSessionSidebarGeometry(t *testing.T) {
	for _, width := range []int{45, 60, 96, 140} {
		m := tabsSessionModel()
		m.width = width
		m.refreshTreeRows()
		w, _ := m.termRegionSize()
		if sw := m.termSidebarWidth(); sw > 0 && w+sw+paneGap != width {
			t.Fatal("PTY size does not reserve sidebar")
		}
		lines := strings.Split(m.View(), "\n")
		if len(lines) != m.height {
			t.Fatalf("%d: height %d", width, len(lines))
		}
		for _, line := range lines {
			if lipgloss.Width(line) != width {
				t.Fatalf("%d: line width %d", width, lipgloss.Width(line))
			}
		}
	}
}

func TestSidebarSelectionSurvivesAttentionReorder(t *testing.T) {
	m := tabsSessionModel()
	// Use session rows: the server's tree builder sorts urgent sessions first.
	m.jobs.tree.tab = sideSpaces
	m.refreshTreeRows()
	for i, r := range m.jobs.tree.rows {
		if r.session == "agent-2" {
			m.jobs.tree.cursor = i
		}
	}
	m.nodes = buildTree([]string{"default", "work"}, []protocol.SessionInfo{
		{Name: "agent-2", Group: "work", Attention: protocol.AttentionError, AttentionSince: time.Now()},
		{Name: "agent-1", Group: "work"},
	}, nil)
	m.refreshTreeRows()
	r, _ := m.jobs.tree.current()
	if r.session != "agent-2" {
		t.Fatal("refresh moved selected session")
	}
}

func TestRenameKeepsLiveLayoutAndScope(t *testing.T) {
	m := tabsSessionModel()
	m.jobs.session = "agent-2"
	root := m.activeRoot()
	nm, _ := m.Update(actionDoneMsg{renamedFrom: "agent-2", renamedTo: "renamed"})
	m = nm.(model)
	if m.activeSession != "renamed" || m.jobs.session != "renamed" || m.activeRoot() != root || m.layouts["agent-2"] != nil || m.shell.session != "renamed" {
		t.Fatal("rename lost local session state")
	}
}

func TestJobsFromSessionKeepsContext(t *testing.T) {
	m := tabsSessionModel()
	m.jobs.tree.tab = sideAgents
	m, _ = m.openJobsView()
	if m.jobs.session != "agent-2" || m.jobs.tree.tab != sideAgents {
		t.Fatal("opening jobs lost session or sidebar context")
	}
}

func TestResizeReleasesHiddenSidebarFocus(t *testing.T) {
	m := tabsSessionModel()
	m.jobs.tree.focus = true
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 45, Height: 20})
	if nm.(model).jobs.tree.focus {
		t.Fatal("hidden sidebar still owns keyboard")
	}
}
