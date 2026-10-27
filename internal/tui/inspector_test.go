package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/han/runic/internal/protocol"
)

func inspectorModel(t *testing.T, pane string) model {
	t.Helper()
	lipgloss.SetColorProfile(termenv.Ascii)
	m := sideTabsModel()
	m.jobs.tree.tab = sideAgents
	m.refreshTreeRows()
	for i, r := range m.jobs.tree.rows {
		if r.pane == pane {
			m.jobs.tree.cursor = i
			return m
		}
	}
	t.Fatalf("no agent row for pane %q", pane)
	return m
}

func inspectorText(t *testing.T, m model, inner int) string {
	t.Helper()
	lines, _, ok := m.sessionDetailLines(inner)
	if !ok {
		t.Fatal("expected a detail panel")
	}
	for i, l := range lines {
		if w := lipgloss.Width(l); w != inner {
			t.Errorf("line %d is %d columns, want %d: %q", i, w, inner, stripAnsi(l))
		}
	}
	return stripAnsi(strings.Join(lines, "\n"))
}

// The panel is ordered by what you need in order to decide what to do. It used
// to lead with the session and pane — true, but you already know which row you
// are on — and left the question competing with metadata.
func TestInspectorLeadsWithStateThenRequest(t *testing.T) {
	m := inspectorModel(t, "p1")
	got := inspectorText(t, m, 70)

	order := []string{"State:", "Asked:", "Do:", "Agent:", "Group:"}
	at := -1
	for _, want := range order {
		i := strings.Index(got, want)
		if i < 0 {
			t.Fatalf("panel is missing %q:\n%s", want, got)
		}
		if i < at {
			t.Errorf("%q comes out of order:\n%s", want, got)
		}
		at = i
	}
}

// A clipped question reads as a different, shorter question, so the request
// wraps instead of truncating.
func TestInspectorWrapsTheRequest(t *testing.T) {
	m := inspectorModel(t, "p1")
	long := "Should this endpoint require authentication, or is it fine to leave it open for the health check?"
	for i := range m.agents {
		if m.agents[i].Pane == "p1" {
			m.agents[i].Note = long
		}
	}
	m.refreshTreeRows()

	got := inspectorText(t, m, 60)
	// Every word of it survives somewhere on the panel.
	for _, word := range []string{"endpoint", "authentication", "health", "check?"} {
		if !strings.Contains(got, word) {
			t.Errorf("the request lost %q:\n%s", word, got)
		}
	}
	if !strings.Contains(got, "Asked:") {
		t.Error("the wrapped field should still carry its label once")
	}
	if strings.Count(got, "Asked:") != 1 {
		t.Error("the label should not repeat on continuation lines")
	}
}

// "done" on its own answers nothing, so the panel shows what the pane printed.
func TestInspectorShowsTheOutputSnippet(t *testing.T) {
	m := inspectorModel(t, "p2")
	r, _ := m.jobs.tree.current()
	m.snippet = agentSnippetState{
		key:   snippetKey(r.session, r.pane),
		lines: []string{"ok \tpkg/one\t1.0s", "FAIL\tpkg/two\t0.2s"},
	}
	got := inspectorText(t, m, 70)
	if !strings.Contains(got, "Output:") {
		t.Fatalf("expected an output section:\n%s", got)
	}
	for _, want := range []string{"pkg/one", "pkg/two"} {
		if !strings.Contains(got, want) {
			t.Errorf("snippet missing %q:\n%s", want, got)
		}
	}
	// Tabs measure as one column and render as several, so they must be gone.
	if strings.Contains(got, "\t") {
		t.Error("tabs should be expanded before the width is budgeted")
	}
}

// A tail that arrived for a pane the cursor has since left must not be shown
// under the new one.
func TestSnippetIsScopedToItsPane(t *testing.T) {
	m := inspectorModel(t, "p2")
	m.snippet = agentSnippetState{key: snippetKey("somewhere", "else"), lines: []string{"stale"}}
	if got := inspectorText(t, m, 70); strings.Contains(got, "stale") {
		t.Errorf("a snippet from another pane leaked in:\n%s", got)
	}

	(&m).applyAgentTail(agentTailMsg{key: snippetKey("somewhere", "else"), lines: []string{"also stale"}})
	if got := inspectorText(t, m, 70); strings.Contains(got, "also stale") {
		t.Error("a reply for a pane we are not on should be dropped")
	}
}

// Moving the cursor asks for the newly selected agent's output, rather than
// waiting up to a second for the next poll to notice.
func TestCursorMoveFetchesSnippet(t *testing.T) {
	m := inspectorModel(t, "p2")
	m.jobs.tree.focus = true
	_, cmd, handled := m.handleTreeKey(keyRune('j'))
	if !handled {
		t.Fatal("the sidebar should handle j")
	}
	if cmd == nil {
		t.Error("moving onto another agent should fetch its output")
	}
}

// A request already in flight is not reissued on every poll.
func TestSnippetRequestedOncePerSelection(t *testing.T) {
	m := inspectorModel(t, "p2")
	if cmd := (&m).requestAgentSnippet(); cmd == nil {
		t.Fatal("the first look at a pane should fetch its output")
	}
	if cmd := (&m).requestAgentSnippet(); cmd != nil {
		t.Error("a request already in flight should not be repeated")
	}
	// Once the answer lands, it is cached rather than re-fetched.
	(&m).applyAgentTail(agentTailMsg{key: m.snippet.key, lines: []string{"done"}})
	if cmd := (&m).requestAgentSnippet(); cmd != nil {
		t.Error("a cached tail should not be re-fetched")
	}
}

// A working agent that has stopped printing is the one failure it cannot report
// itself, so the panel and the row both say so.
func TestStalledAgentIsReported(t *testing.T) {
	m := sideTabsModel()
	for i := range m.agents {
		if m.agents[i].Pane == "p1" && m.agents[i].Session == "default" {
			m.agents[i].Attention = protocol.AttentionWorking
			m.agents[i].LastOutput = time.Now().Add(-20 * time.Minute)
		}
	}
	m.jobs.tree.tab = sideAgents
	m.refreshTreeRows()

	var row treeRow
	for i, r := range m.jobs.tree.rows {
		if r.session == "default" && r.pane == "p1" {
			row, m.jobs.tree.cursor = r, i
		}
	}
	if !agentStalled(row) {
		t.Fatal("twenty minutes of silence while working should count as stalled")
	}
	if line := stripAnsi(m.renderTreeRow(row, false, treePaneWidth)); !strings.Contains(line, "QUIET") {
		t.Errorf("the row should report the silence: %q", line)
	}
	if got := inspectorText(t, m, 70); !strings.Contains(got, "quiet for") {
		t.Errorf("the panel should report the silence:\n%s", got)
	}
}

// A working agent that is printing is not stalled, and a waiting one is never
// "quiet" — it is waiting, which it told us.
func TestHealthyAndWaitingAgentsAreNotStalled(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		row  treeRow
	}{
		{"recently printed", treeRow{attention: protocol.AttentionWorking, lastOutput: now.Add(-time.Second)}},
		{"never printed", treeRow{attention: protocol.AttentionWorking}},
		{"waiting", treeRow{attention: protocol.AttentionWaiting, lastOutput: now.Add(-time.Hour)}},
		{"errored", treeRow{attention: protocol.AttentionError, lastOutput: now.Add(-time.Hour)}},
		{"idle", treeRow{attention: protocol.AttentionIdle, lastOutput: now.Add(-time.Hour)}},
	}
	for _, c := range cases {
		if agentStalled(c.row) {
			t.Errorf("%s should not be stalled", c.name)
		}
	}
}
