package tui

import (
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
			if m.jobs.session != "" && s.Name != m.jobs.session {
				continue
			}
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

// The Jobs pane header is drawn by renderMainTabStrip (agents_view.go), which
// puts the primary views first and these state filters after them. There is no
// second renderer for this strip: one that is never installed drifts from the
// one that is, and then a click lands on whatever is actually there.
