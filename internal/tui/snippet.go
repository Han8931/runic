package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/han/runic/internal/client"
)

// What an agent printed, shown in the inspector when you select it.
//
// "done" on its own answers nothing. The Stop hook fires as the agent exits and
// carries no message, so a finished agent's row says only that it finished —
// and finding out what it did meant attaching to the pane and scrolling. The
// daemon already keeps a backlog ring per pane to replay on attach, so the tail
// of it is available for the asking.
//
// It is fetched for the selected pane only, once per selection, rather than
// riding along with the one-second poll: this is a few hundred lines of text
// per request, and nothing wants it for the forty panes you are not looking at.

// snippetLines is how much of the tail the panel shows. Enough for a test
// summary or a stack trace's head; short enough to leave room for the fields
// under it.
const snippetLines = 6

// agentSnippetState caches the tail for one pane.
type agentSnippetState struct {
	// key identifies the pane the lines belong to, so a stale reply for the row
	// you just moved off is discarded rather than shown under the new one.
	key   string
	lines []string
	// asked marks a request in flight, so a repeated render does not fire one
	// request per frame.
	asked bool
}

type agentTailMsg struct {
	key   string
	lines []string
	err   error
}

// snippetKey identifies a pane for caching purposes.
func snippetKey(session, pane string) string { return session + "\x00" + pane }

// agentSnippet returns the cached tail for an agent row, if it is for that row.
func (m model) agentSnippet(r treeRow) []string {
	if m.snippet.key != snippetKey(r.session, r.pane) {
		return nil
	}
	return m.snippet.lines
}

// requestAgentSnippet asks for the selected agent's tail when the selection has
// moved to a pane whose output is not already cached. It returns nil when there
// is nothing to do, which is the common case — the selection does not move on
// most polls.
func (m *model) requestAgentSnippet() tea.Cmd {
	r, ok := m.jobs.tree.current()
	if !ok || r.kind != treeAgent || r.pane == "" {
		m.snippet = agentSnippetState{}
		return nil
	}
	key := snippetKey(r.session, r.pane)
	if m.snippet.key == key && (m.snippet.asked || m.snippet.lines != nil) {
		return nil
	}
	m.snippet = agentSnippetState{key: key, asked: true}
	session, pane := r.session, r.pane
	return func() tea.Msg {
		lines, err := client.TerminalTail(session, pane, snippetLines)
		return agentTailMsg{key: snippetKey(session, pane), lines: lines, err: err}
	}
}

// applyAgentTail stores a tail reply, ignoring one that arrived for a pane the
// cursor has already left.
func (m *model) applyAgentTail(msg agentTailMsg) {
	if msg.key != m.snippet.key {
		return
	}
	m.snippet.asked = false
	if msg.err != nil {
		// A pane that died between the request and the reply is not worth
		// reporting: the row itself is about to say so.
		m.snippet.lines = nil
		return
	}
	// Blank-only output is no output.
	var kept []string
	for _, l := range msg.lines {
		if strings.TrimSpace(l) != "" {
			kept = append(kept, l)
		}
	}
	m.snippet.lines = kept
}
