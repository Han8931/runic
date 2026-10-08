package tui

import (
	"errors"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/han/runic/internal/client"
)

// tea.Cmd wrappers around the client's session and group actions. Each runs off
// the Update goroutine and reports back as an actionDoneMsg.

// sessionExists reports whether a session by that name is present in the tree.
func (m model) sessionExists(name string) bool {
	for _, n := range m.nodes {
		for _, s := range n.sessions {
			if s.Name == name {
				return true
			}
		}
	}
	return false
}

func createSession(name string) tea.Cmd {
	return func() tea.Msg {
		err := client.SessionCreate(name)
		if err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{status: fmt.Sprintf("created session %q", name)}
	}
}

// createSessionInGroup creates a session and, when branch is non-empty, gives it
// an isolated git worktree on that branch (branched from the directory `ru` was
// started in).
func createSessionInGroup(name, group, branch string) tea.Cmd {
	return func() tea.Msg {
		if err := client.SessionCreateWorktree(name, branch != "", "", branch); err != nil {
			return actionDoneMsg{err: err}
		}
		if group != "" && group != "default" {
			// The group may not exist yet; create it first (ignoring an
			// "already exists" error) so the move below can land.
			_ = client.GroupCreate(group)
			if err := client.SessionMove(name, group); err != nil {
				return actionDoneMsg{err: err}
			}
			return actionDoneMsg{status: fmt.Sprintf("created session %q in group %q", name, group)}
		}
		return actionDoneMsg{status: fmt.Sprintf("created session %q", name)}
	}
}

// deleteSession deletes a session, keeping its worktree if an agent left
// uncommitted work there. That refusal comes back as offerDiscard, which puts
// the TUI into a second confirmation rather than silently losing the work or
// silently keeping a directory nobody owns any more.
func deleteSession(name string) tea.Cmd {
	return func() tea.Msg {
		err := client.SessionDelete(name)
		var dirty *client.DirtyWorktreeError
		if errors.As(err, &dirty) {
			return actionDoneMsg{err: err, offerDiscard: name, detail: dirty.Detail}
		}
		if err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{status: fmt.Sprintf("deleted session %q", name)}
	}
}

// discardSession is the confirmed second attempt: delete the session and throw
// away the uncommitted work in its worktree.
func discardSession(name string) tea.Cmd {
	return func() tea.Msg {
		if err := client.SessionDeleteDiscard(name, true); err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{status: fmt.Sprintf("deleted session %q and discarded its uncommitted work", name)}
	}
}

func renameSession(oldName, newName string) tea.Cmd {
	return func() tea.Msg {
		err := client.SessionRename(oldName, newName)
		if err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{status: fmt.Sprintf("renamed %q -> %q", oldName, newName)}
	}
}

func deleteGroup(name string) tea.Cmd {
	return func() tea.Msg {
		err := client.GroupDelete(name)
		if err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{status: fmt.Sprintf("deleted group %q", name)}
	}
}

func moveSession(session, group string) tea.Cmd {
	return func() tea.Msg {
		err := client.SessionMove(session, group)
		if err != nil {
			return actionDoneMsg{err: err}
		}
		return actionDoneMsg{status: fmt.Sprintf("moved %q to %q", session, group)}
	}
}

// editSession renames and/or moves a session in one sequential command so the
// rename lands before the move references the new name.
func editSession(oldName, newName, oldGroup, newGroup string) tea.Cmd {
	return func() tea.Msg {
		cur := oldName
		if newName != "" && newName != oldName {
			if err := client.SessionRename(oldName, newName); err != nil {
				return actionDoneMsg{err: err}
			}
			cur = newName
		}
		if newGroup != oldGroup {
			g := newGroup
			if g == "" {
				g = "default"
			}
			if g != "default" {
				_ = client.GroupCreate(g) // create the target group if missing
			}
			if err := client.SessionMove(cur, g); err != nil {
				return actionDoneMsg{err: err}
			}
		}
		return actionDoneMsg{status: fmt.Sprintf("updated session %q", cur)}
	}
}
