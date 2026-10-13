package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/han/runic/internal/protocol"
)

// jobsTab is one view of the job table, shown as a tab on the Jobs pane and
// rolled with Tab / Shift+Tab. Each tab narrows the table to one state bucket;
// the filter and sort apply on top of it.
type jobsTab int

const (
	tabAll jobsTab = iota
	tabRunning
	tabQueued
	tabDone
	tabFailed
	numJobsTabs
)

func (t jobsTab) label() string {
	switch t {
	case tabRunning:
		return "Running"
	case tabQueued:
		return "Queued"
	case tabDone:
		return "Done"
	case tabFailed:
		return "Failed"
	default:
		return "All"
	}
}

// matches reports whether a job belongs in the tab. The buckets are the same
// ones jobStateCounts tallies, so a tab always holds what its count says.
func (t jobsTab) matches(j protocol.JobInfo) bool {
	if t == tabAll {
		return true
	}
	return jobBucket(j) == t
}

// jobBucket files a job under the one state tab it belongs to.
func jobBucket(j protocol.JobInfo) jobsTab {
	switch j.State {
	case protocol.StateRunning:
		return tabRunning
	case protocol.StateQueued, protocol.StateAllocating:
		return tabQueued
	case protocol.StateFinished:
		if !j.Result.Canceled && (j.Result.DiedBySignal || j.Result.ExitCode != 0) {
			return tabFailed
		}
	}
	return tabDone
}

// emptyMessage tells the user why a tab is empty and how it gets filled.
func (t jobsTab) emptyMessage() (headline, hint string) {
	switch t {
	case tabRunning:
		return "nothing running", "tab rolls to the next view"
	case tabQueued:
		return "the queue is empty", "submit a job with  ru <command>"
	case tabDone:
		return "nothing finished yet", "tab rolls to the next view"
	case tabFailed:
		return "no failures", "tab rolls to the next view"
	}
	return "no jobs yet", "submit one with  ru <command>   ·  ? for help"
}

// cycleTab rolls the view delta tabs left or right, wrapping around. The cursor
// stays anchored to its job when that job is still visible in the new tab.
func (m *model) cycleTab(delta int) {
	n := int(numJobsTabs)
	m.switchTab(jobsTab(((int(m.jobs.tab)+delta)%n + n) % n))
}

func (m *model) switchTab(t jobsTab) {
	if t == m.jobs.tab || t < 0 || t >= numJobsTabs {
		return
	}
	m.jobs.tab = t
	m.jobs.visual = false
	m.jobs.offset = 0
	m.refreshJobsRows()
}

// tabCount is the number of jobs a tab holds under the current filter.
func (m model) tabCount(t jobsTab) int {
	n := 0
	for _, node := range m.nodes {
		for _, s := range node.sessions {
			for _, j := range filterJobs(node.jobs[s.Name], m.jobs.filter, "", true) {
				if t.matches(j) {
					n++
				}
			}
		}
	}
	return n
}

// tabSpan is where one tab landed in the rendered strip, in columns relative to
// the strip's left edge, for mouse hit-testing.
type tabSpan struct {
	tab        jobsTab
	start, end int // [start, end)
}

// renderTabStrip draws the Jobs pane header in gorae's style: the active tab is
// a filled chip carrying its job count, the rest are plain names. The Failed tab
// keeps its count visible even when inactive, in red, because a failure is the
// one thing worth noticing from another view.
func (m model) renderTabStrip(width int) (string, []tabSpan) {
	var b strings.Builder
	var spans []tabSpan
	col := 0
	write := func(s string) {
		b.WriteString(s)
		col += lipgloss.Width(s)
	}
	write(" ")
	for t := tabAll; t < numJobsTabs; t++ {
		if t > tabAll {
			write("  ")
		}
		start := col
		switch {
		case t == m.jobs.tab:
			write(chipStyle.Render(fmt.Sprintf(" %s %d ", t.label(), m.tabCount(t))))
		case t == tabFailed:
			if n := m.tabCount(t); n > 0 {
				write(tabInactiveStyle.Render(t.label()+" ") + finishedErrStyle.Render(fmt.Sprint(n)))
			} else {
				write(tabInactiveStyle.Render(t.label()))
			}
		default:
			write(tabInactiveStyle.Render(t.label()))
		}
		spans = append(spans, tabSpan{tab: t, start: start, end: col})
	}
	return fitToWidth(b.String(), width), spans
}
