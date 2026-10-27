package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/han/runic/internal/protocol"
)

// tabsModel holds one job in every state bucket.
func tabsModel() model {
	sessions := []protocol.SessionInfo{{Name: "default", Group: "default"}}
	jobs := []protocol.JobInfo{
		{ID: 1, Session: "default", Command: "make", State: protocol.StateRunning},
		{ID: 2, Session: "default", Command: "test", State: protocol.StateQueued},
		{ID: 3, Session: "default", Command: "ok", State: protocol.StateFinished},
		{ID: 4, Session: "default", Command: "bad", State: protocol.StateFinished,
			Result: protocol.Result{ExitCode: 2}},
		{ID: 5, Session: "default", Command: "stop", State: protocol.StateFinished,
			Result: protocol.Result{ExitCode: 1, Canceled: true}},
	}
	m := model{viewMode: viewJobs, width: 160, height: 40}
	m.nodes = buildTree([]string{"default"}, sessions, jobs)
	m.jobs.allJobs = jobs
	m.refreshJobsRows()
	return m
}

func rowIDs(m model) []int {
	var ids []int
	for _, r := range m.jobs.rows {
		ids = append(ids, r.job.ID)
	}
	return ids
}

// Each tab narrows the table to its state bucket; a canceled job counts as
// done, not failed.
func TestJobsTabsFilterRows(t *testing.T) {
	m := tabsModel()
	want := map[jobsTab]string{
		tabAll: "[1 2 3 4 5]", tabRunning: "[1]", tabQueued: "[2]", tabDone: "[3 5]", tabFailed: "[4]",
	}
	for tab := tabAll; tab < numJobsTabs; tab++ {
		m.jobs.tab = tab
		m.refreshJobsRows()
		if got := fmt.Sprint(rowIDs(m)); got != want[tab] {
			t.Errorf("%s: rows %s, want %s", tab.label(), got, want[tab])
		}
		if n := m.tabCount(tab); n != len(m.jobs.rows) {
			t.Errorf("%s: count %d but %d rows", tab.label(), n, len(m.jobs.rows))
		}
	}
}

// ←/→ roll the job state filters and wrap at both ends. They are a level below
// a view — Tab rolls the primary views, Jobs and Agents — so the filters have
// the arrows to themselves.
func TestJobsFilterCycles(t *testing.T) {
	m := tabsModel()
	press := func(k tea.KeyMsg) {
		nm, _ := m.handleJobsKey(k)
		m = nm.(model)
	}
	for i := 1; i < int(numJobsTabs); i++ {
		press(tea.KeyMsg{Type: tea.KeyRight})
		if m.jobs.tab != jobsTab(i) {
			t.Fatalf("after %d rights: view %v", i, m.jobs.tab)
		}
	}
	press(tea.KeyMsg{Type: tea.KeyRight})
	if m.jobs.tab != tabAll {
		t.Fatalf("past the last filter should wrap to All, got %v", m.jobs.tab)
	}
	press(tea.KeyMsg{Type: tea.KeyLeft})
	if m.jobs.tab != tabFailed {
		t.Fatalf("left from All should wrap to Failed, got %v", m.jobs.tab)
	}

	// Tab is the primary-view key, and must not disturb the filter.
	before := m.jobs.tab
	press(tea.KeyMsg{Type: tea.KeyTab})
	if m.jobs.main != mainAgents {
		t.Errorf("tab should switch to Agents, got %v", m.jobs.main)
	}
	if m.jobs.tab != before {
		t.Errorf("switching view changed the job filter: %v -> %v", before, m.jobs.tab)
	}
	press(tea.KeyMsg{Type: tea.KeyTab})
	if m.jobs.main != mainJobs {
		t.Errorf("tab again should come back to Jobs, got %v", m.jobs.main)
	}

	// The arrows belong to Jobs: in Agents there is nothing to filter, so they
	// must not quietly change hidden state.
	press(tea.KeyMsg{Type: tea.KeyTab}) // -> Agents
	before = m.jobs.tab
	press(tea.KeyMsg{Type: tea.KeyRight})
	if m.jobs.tab != before {
		t.Error("the arrows should be inert while Agents is showing")
	}
}

// The view survives leaving and re-entering the management screen.
func TestJobsTabSurvivesReopen(t *testing.T) {
	m := tabsModel()
	m.jobs.tab = tabQueued
	m, _ = m.openJobsView()
	if m.jobs.tab != tabQueued {
		t.Fatalf("reopen reset the view to %v", m.jobs.tab)
	}
}

// Every rendered row spans exactly the terminal width, with or without the
// sidebar, and the Details pane always sits under the table.
func TestJobsViewFillsScreen(t *testing.T) {
	for _, tc := range []struct {
		w, h int
		tree bool
	}{
		{180, 40, true},
		{110, 40, true},
		{140, 30, false},
		{60, 24, true},
	} {
		m := tabsModel()
		m.width, m.height = tc.w, tc.h
		m.jobs.tree.show = tc.tree
		m.refreshJobsRows()
		if l := m.jobsLayout(); l.detailH == 0 || l.detailW != l.listW || l.listH+l.detailH != l.paneH {
			t.Errorf("%dx%d tree=%v: details should fill the space under the table: %+v", tc.w, tc.h, tc.tree, l)
		}
		lines := strings.Split(m.renderJobsView(tc.w, tc.h), "\n")
		if len(lines) != tc.h {
			t.Fatalf("%dx%d: %d lines", tc.w, tc.h, len(lines))
		}
		for i, l := range lines {
			if w := lipgloss.Width(l); w != tc.w {
				t.Errorf("%dx%d tree=%v line %d: width %d: %q", tc.w, tc.h, tc.tree, i, w, stripAnsi(l))
			}
		}
		if !strings.Contains(stripAnsi(lines[mgmtTabStripLine]), " All 5 ") {
			t.Errorf("tab strip row should carry the active tab chip: %q", stripAnsi(lines[mgmtTabStripLine]))
		}
	}
}

// Clicking a tab in the strip switches to it.
func TestJobsTabClick(t *testing.T) {
	m := tabsModel()
	m.jobs.tree.show = true
	l := m.jobsLayout()
	_, spans, _ := m.renderMainTabStrip(l.listW - 2)
	target := spans[tabFailed]
	nm, _ := m.handleMouse(tea.MouseMsg{
		X: l.listX + 1 + target.start, Y: mgmtTabStripLine,
		Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
	})
	if got := nm.(model).jobs.tab; got != tabFailed {
		t.Fatalf("click on the Failed tab gave view %v", got)
	}
}
