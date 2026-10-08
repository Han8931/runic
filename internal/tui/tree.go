package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/han/runic/internal/protocol"
)

type treeNode struct {
	group    string
	expanded bool
	sessions []protocol.SessionInfo
	jobs     map[string][]protocol.JobInfo
}

func buildTree(groups []string, sessions []protocol.SessionInfo, jobs []protocol.JobInfo) []treeNode {
	bySession := make(map[string][]protocol.JobInfo)
	for _, j := range jobs {
		bySession[j.Session] = append(bySession[j.Session], j)
	}

	byGroup := make(map[string][]protocol.SessionInfo)
	for _, session := range sessions {
		group := session.Group
		if group == "" {
			group = "default"
		}
		byGroup[group] = append(byGroup[group], session)
	}
	for group := range byGroup {
		sortSessionsByAttention(byGroup[group])
	}

	nodes := make([]treeNode, 0, len(groups))
	for _, group := range groups {
		nodes = append(nodes, treeNode{
			group:    group,
			expanded: true,
			sessions: byGroup[group],
			jobs:     bySession,
		})
	}
	return nodes
}

func groupSummary(node treeNode) string {
	totalJobs := 0
	running := 0
	queued := 0
	for _, session := range node.sessions {
		jobs := node.jobs[session.Name]
		totalJobs += len(jobs)
		r, q, _ := jobCounts(jobs)
		running += r
		queued += q
	}
	if len(node.sessions) == 0 {
		return "empty"
	}
	var parts []string
	parts = append(parts, fmt.Sprintf("%d session%s", len(node.sessions), plural(len(node.sessions))))
	if running > 0 {
		parts = append(parts, fmt.Sprintf("%d running", running))
	}
	if queued > 0 {
		parts = append(parts, fmt.Sprintf("%d queued", queued))
	}
	if totalJobs > 0 && running == 0 && queued == 0 {
		parts = append(parts, fmt.Sprintf("%d jobs", totalJobs))
	}
	return strings.Join(parts, " · ")
}

// sortSessionsByAttention floats the sessions that need you — a pane blocked on
// input, or one that stopped on an error — to the top of their group, most
// urgent first; everything else keeps the daemon's name order. Only "needs you"
// states reorder, so a session merely working away never shuffles itself under
// your cursor.
func sortSessionsByAttention(sessions []protocol.SessionInfo) {
	rank := func(s protocol.SessionInfo) protocol.Attention {
		if s.Attention.NeedsYou() {
			return s.Attention
		}
		return protocol.AttentionNone
	}
	sort.SliceStable(sessions, func(i, j int) bool {
		return rank(sessions[i]) > rank(sessions[j])
	})
}

// attentionMark returns the one-column glyph for a session's attention state
// and the style that colors its row. A session with nothing to report renders
// exactly as before.
func attentionMark(a protocol.Attention) (string, lipgloss.Style, bool) {
	switch a {
	case protocol.AttentionWaiting:
		return "●", attnWaitingStyle, true
	case protocol.AttentionError:
		return "×", attnErrorStyle, true
	case protocol.AttentionWorking:
		return "◐", attnWorkingStyle, true
	default:
		return " ", sessionStyle, false
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// jobStateText returns a job's state label together with the style that colors
// it. Splitting the two lets callers recompose the label onto a highlighted
// background while keeping the semantic foreground (e.g. red for a failure).
//
// The labels are uppercase: in a column of short words, caps read as a status
// code rather than prose, and the word itself carries the meaning for anyone
// who cannot tell the hues apart. No glyphs — a column of symbols next to a
// column of words says the same thing twice.
func jobStateText(j protocol.JobInfo) (string, lipgloss.Style) {
	switch j.State {
	case protocol.StateRunning:
		return "RUNNING", runningStyle
	case protocol.StateQueued:
		if j.Attempt > 0 {
			return fmt.Sprintf("RETRY %d/%d", j.Attempt, j.Retries), queuedStyle
		}
		return "QUEUED", queuedStyle
	case protocol.StateAllocating:
		return "WAITING", queuedStyle
	case protocol.StateFinished:
		// A job you killed is not a failure: it gets the muted CANCELED label
		// rather than the red signal/exit one, and the footer's fail count skips
		// it (see jobStateCounts).
		if j.Result.Canceled {
			return "CANCELED", skippedStyle
		}
		if j.Result.TimedOut {
			return "TIMEOUT", finishedErrStyle
		}
		if j.Result.DiedBySignal {
			return fmt.Sprintf("SIGNAL %d", j.Result.Signal), finishedErrStyle
		}
		if j.Result.ExitCode != 0 {
			return fmt.Sprintf("EXIT %d", j.Result.ExitCode), finishedErrStyle
		}
		return "DONE", finishedStyle
	case protocol.StateSkipped:
		return "SKIPPED", skippedStyle
	default:
		return "UNKNOWN", lipgloss.NewStyle()
	}
}

func sessionSummary(jobs []protocol.JobInfo) string {
	if len(jobs) == 0 {
		return "(empty)"
	}
	running, queued, finished := 0, 0, 0
	for _, j := range jobs {
		switch j.State {
		case protocol.StateRunning:
			running++
		case protocol.StateQueued:
			queued++
		default:
			finished++
		}
	}

	var parts []string
	if running > 0 {
		parts = append(parts, fmt.Sprintf("%d running", running))
	}
	if queued > 0 {
		parts = append(parts, fmt.Sprintf("%d queued", queued))
	}
	if finished > 0 {
		parts = append(parts, fmt.Sprintf("%d done", finished))
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

func jobCounts(jobs []protocol.JobInfo) (running, queued, finished int) {
	for _, j := range jobs {
		switch j.State {
		case protocol.StateRunning:
			running++
		case protocol.StateQueued:
			queued++
		default:
			finished++
		}
	}
	return
}
