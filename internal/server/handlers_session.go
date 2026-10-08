package server

import (
	"fmt"
	"log"
	"net"

	"github.com/han/runic/internal/protocol"
)

// Request handlers for sessions and groups, including the combined tree view
// the TUI polls.

func (s *Server) handleSessionList(conn net.Conn) bool {
	return s.sendMsg(conn, &protocol.Msg{
		Type:    protocol.MsgSessionListOK,
		Payload: protocol.PayloadSessionList{Sessions: s.jobs.AllSessions()},
	})
}

func (s *Server) handleSessionCreate(conn net.Conn, msg *protocol.Msg) bool {
	payload, err := protocol.PayloadAs[protocol.PayloadSession](msg)
	if err != nil {
		return s.sendError(conn, err.Error())
	}
	if !s.jobs.CreateSession(payload.Name) {
		return s.sendError(conn, "session already exists")
	}
	if payload.Worktree {
		wt, err := addWorktree(payload.Name, payload.Repo, payload.Branch)
		if err != nil {
			// Don't leave a half-made session behind: roll the create back so a
			// retry (after fixing the repo path, say) starts clean.
			s.jobs.DeleteSession(payload.Name)
			return s.sendError(conn, err.Error())
		}
		s.jobs.SetSessionWorktree(payload.Name, wt)
		log.Printf("session %s: worktree %s on branch %s", payload.Name, wt.Path, wt.Branch)
	}
	return s.sendMsg(conn, &protocol.Msg{Type: protocol.MsgSessionCreateOK})
}

func (s *Server) handleSessionRename(conn net.Conn, msg *protocol.Msg) bool {
	payload, err := protocol.PayloadAs[protocol.PayloadSessionRename](msg)
	if err != nil {
		return s.sendError(conn, err.Error())
	}
	if !s.jobs.RenameSession(payload.OldName, payload.NewName) {
		return s.sendError(conn, "cannot rename session")
	}
	s.terminals.RenameSession(payload.OldName, payload.NewName)
	return s.sendMsg(conn, &protocol.Msg{Type: protocol.MsgSessionRenameOK})
}

func (s *Server) handleSessionDelete(conn net.Conn, msg *protocol.Msg) bool {
	payload, err := protocol.PayloadAs[protocol.PayloadSession](msg)
	if err != nil {
		return s.sendError(conn, err.Error())
	}
	// Capture the worktree before the delete drops the record, then remove it
	// once the session is really gone and its panes are dead — a live pane
	// holding the directory open would make `git worktree remove` fail.
	wt, hasWorktree := s.jobs.SessionWorktreeFor(payload.Name)
	// Uncommitted work is checked *before* the session is touched, so a refusal
	// leaves everything exactly as it was and the user can retry with Discard.
	// removeWorktree checks again under the real conditions (panes dead); that
	// second refusal can only report a leftover directory, since by then the
	// session is already gone.
	if hasWorktree && !payload.Discard {
		if entries := worktreeChanges(wt.Path); len(entries) > 0 {
			return s.sendError(conn, (&DirtyWorktreeError{Path: wt.Path, Entries: entries}).Error())
		}
	}
	ok, reason := s.jobs.DeleteSession(payload.Name)
	if !ok {
		return s.sendError(conn, reason)
	}
	s.terminals.DropSession(payload.Name)
	if hasWorktree {
		if err := removeWorktree(wt, payload.Discard); err != nil {
			// The session is gone either way; report the leftover directory
			// rather than failing a delete that already happened.
			log.Printf("session %s: remove worktree %s: %v", payload.Name, wt.Path, err)
			return s.sendError(conn, fmt.Sprintf("session deleted, but its worktree %s was kept: %v", wt.Path, err))
		}
		log.Printf("session %s: removed worktree %s (branch %s kept)", payload.Name, wt.Path, wt.Branch)
	}
	return s.sendMsg(conn, &protocol.Msg{Type: protocol.MsgSessionDeleteOK})
}

func (s *Server) handleListSession(conn net.Conn, msg *protocol.Msg) bool {
	payload, err := protocol.PayloadAs[protocol.PayloadSession](msg)
	if err != nil {
		return s.sendError(conn, err.Error())
	}
	jobs := s.jobs.AllInfoBySession(payload.Name)
	for _, info := range jobs {
		if !s.sendMsg(conn, &protocol.Msg{
			Type:    protocol.MsgListLine,
			Payload: protocol.PayloadListLine{Job: info},
		}) {
			return false
		}
	}
	return s.sendMsg(conn, &protocol.Msg{
		Type:    protocol.MsgListEnd,
		Payload: protocol.PayloadSlots{Slots: s.scheduler.MaxSlots()},
	})
}

func (s *Server) handleClearFinishedSession(conn net.Conn, msg *protocol.Msg) bool {
	payload, err := protocol.PayloadAs[protocol.PayloadSession](msg)
	if err != nil {
		return s.sendError(conn, err.Error())
	}
	s.jobs.ClearFinishedInSession(payload.Name)
	return s.sendMsg(conn, &protocol.Msg{Type: protocol.MsgActionOK})
}

func (s *Server) handleTreeList(conn net.Conn) bool {
	s.mu.Lock()
	openJobsView := s.pendingJobsView
	s.pendingJobsView = false
	s.mu.Unlock()
	// Fold each session's pane attention into its SessionInfo so the TUI gets
	// the inbox signal in the same poll as the hierarchy, with no extra trip.
	sessions := s.jobs.AllSessionInfo()
	attention := s.terminals.SessionAttention()
	for i := range sessions {
		if a, ok := attention[sessions[i].Name]; ok {
			sessions[i].Attention = a.Attention
			sessions[i].AttentionNote = a.AttentionNote
			sessions[i].AttentionSince = a.AttentionSince
		}
	}
	return s.sendMsg(conn, &protocol.Msg{
		Type: protocol.MsgTreeListOK,
		Payload: protocol.PayloadTreeData{
			Groups:       s.jobs.AllGroups(),
			Sessions:     sessions,
			Jobs:         s.jobs.AllInfo(),
			MaxSlots:     s.scheduler.MaxSlots(),
			OpenJobsView: openJobsView,
		},
	})
}

func (s *Server) handleRequestJobsView(conn net.Conn) bool {
	s.mu.Lock()
	s.pendingJobsView = true
	s.mu.Unlock()
	return s.sendMsg(conn, &protocol.Msg{Type: protocol.MsgRequestJobsViewOK})
}

func (s *Server) handleGroupList(conn net.Conn) bool {
	return s.sendMsg(conn, &protocol.Msg{
		Type:    protocol.MsgGroupListOK,
		Payload: protocol.PayloadGroupList{Groups: s.jobs.AllGroups()},
	})
}

func (s *Server) handleGroupCreate(conn net.Conn, msg *protocol.Msg) bool {
	payload, err := protocol.PayloadAs[protocol.PayloadSession](msg)
	if err != nil {
		return s.sendError(conn, err.Error())
	}
	if !s.jobs.CreateGroup(payload.Name) {
		return s.sendError(conn, "cannot create group")
	}
	return s.sendMsg(conn, &protocol.Msg{Type: protocol.MsgGroupCreateOK})
}

func (s *Server) handleGroupRename(conn net.Conn, msg *protocol.Msg) bool {
	payload, err := protocol.PayloadAs[protocol.PayloadSessionRename](msg)
	if err != nil {
		return s.sendError(conn, err.Error())
	}
	if !s.jobs.RenameGroup(payload.OldName, payload.NewName) {
		return s.sendError(conn, "cannot rename group")
	}
	return s.sendMsg(conn, &protocol.Msg{Type: protocol.MsgGroupRenameOK})
}

func (s *Server) handleGroupDelete(conn net.Conn, msg *protocol.Msg) bool {
	payload, err := protocol.PayloadAs[protocol.PayloadSession](msg)
	if err != nil {
		return s.sendError(conn, err.Error())
	}
	ok, reason := s.jobs.DeleteGroup(payload.Name)
	if !ok {
		return s.sendError(conn, reason)
	}
	return s.sendMsg(conn, &protocol.Msg{Type: protocol.MsgGroupDeleteOK})
}

func (s *Server) handleSessionMove(conn net.Conn, msg *protocol.Msg) bool {
	payload, err := protocol.PayloadAs[protocol.PayloadSessionMove](msg)
	if err != nil {
		return s.sendError(conn, err.Error())
	}
	if !s.jobs.MoveSession(payload.Session, payload.Group) {
		return s.sendError(conn, "cannot move session")
	}
	return s.sendMsg(conn, &protocol.Msg{Type: protocol.MsgSessionMoveOK})
}
