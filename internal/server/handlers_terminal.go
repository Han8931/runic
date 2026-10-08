package server

import (
	"fmt"
	"net"

	"github.com/han/runic/internal/protocol"
)

// Request handlers for daemon-hosted panes: attach/detach streaming, pane
// creation and killing, and the per-session layout blob the TUI persists.

func (s *Server) handleTerminalAttach(conn net.Conn, msg *protocol.Msg) bool {
	// An attach connection stays open for the pane's whole lifetime. Don't let
	// it count against maxConns, or a few panes plus the TUI's periodic tree
	// polls would exhaust the cap and get silently dropped. Balance the
	// accept-loop's deferred Add(-1) by releasing our slot now and reclaiming
	// it on return.
	s.activeConns.Add(-1)
	defer s.activeConns.Add(1)

	payload, err := protocol.PayloadAs[protocol.PayloadTerminalAttach](msg)
	if err != nil {
		return s.sendError(conn, err.Error())
	}
	term, err := s.terminals.GetOrCreate(payload.Session, payload.Pane, payload.Cols, payload.Rows)
	if err != nil {
		return s.sendError(conn, fmt.Sprintf("terminal: %v", err))
	}
	// Attaching is how you acknowledge a pane's alert: clear a "needs you"
	// state so working through the inbox empties it.
	term.fire(term.seen())

	ch, backlog := term.Attach()
	defer term.Detach(ch)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			in, err := protocol.Recv(conn)
			if err != nil {
				return
			}
			switch in.Type {
			case protocol.MsgTerminalInput:
				p, pErr := protocol.PayloadAs[protocol.PayloadTerminalData](in)
				if pErr == nil {
					term.Write(p.Data)
				}
			case protocol.MsgTerminalResize:
				p, pErr := protocol.PayloadAs[protocol.PayloadTerminalResize](in)
				if pErr == nil {
					term.Resize(p.Cols, p.Rows)
				}
			}
		}
	}()

	if len(backlog) > 0 {
		if !s.sendMsg(conn, &protocol.Msg{Type: protocol.MsgTerminalOutput, Payload: protocol.PayloadTerminalData{Data: backlog}}) {
			return false
		}
	}
	for {
		select {
		case data, ok := <-ch:
			if !ok {
				// Channel closed: distinguish a real shell exit from a
				// forced disconnect of a wedged subscriber. Only the former
				// gets MsgTerminalExit; the latter just drops the connection
				// so the client reattaches instead of marking the pane dead.
				if term.isDone() {
					s.sendMsg(conn, &protocol.Msg{Type: protocol.MsgTerminalExit})
				}
				return false
			}
			if !s.sendMsg(conn, &protocol.Msg{Type: protocol.MsgTerminalOutput, Payload: protocol.PayloadTerminalData{Data: data}}) {
				return false
			}
		case <-done:
			return false
		}
	}
}

func (s *Server) handleTerminalKill(conn net.Conn, msg *protocol.Msg) bool {
	payload, err := protocol.PayloadAs[protocol.PayloadTerminalKill](msg)
	if err != nil {
		return s.sendError(conn, err.Error())
	}
	s.terminals.Kill(payload.Session, payload.Pane)
	return s.sendMsg(conn, &protocol.Msg{Type: protocol.MsgActionOK})
}

func (s *Server) handleTerminalOpen(conn net.Conn, msg *protocol.Msg) bool {
	payload, err := protocol.PayloadAs[protocol.PayloadTerminalOpen](msg)
	if err != nil {
		return s.sendError(conn, err.Error())
	}
	pane, err := s.terminals.Open(payload.Session, payload.Cols, payload.Rows)
	if err != nil {
		return s.sendError(conn, fmt.Sprintf("terminal: %v", err))
	}
	return s.sendMsg(conn, &protocol.Msg{
		Type:    protocol.MsgTerminalOpenOK,
		Payload: protocol.PayloadTerminalName{Pane: pane},
	})
}

func (s *Server) handleTerminalGetLayout(conn net.Conn, msg *protocol.Msg) bool {
	payload, err := protocol.PayloadAs[protocol.PayloadTerminalGetLayout](msg)
	if err != nil {
		return s.sendError(conn, err.Error())
	}
	blob, alive := s.terminals.ListLayout(payload.Session)
	return s.sendMsg(conn, &protocol.Msg{
		Type:    protocol.MsgTerminalLayout,
		Payload: protocol.PayloadTerminalLayout{Blob: blob, Alive: alive},
	})
}

func (s *Server) handleTerminalSetLayout(conn net.Conn, msg *protocol.Msg) bool {
	payload, err := protocol.PayloadAs[protocol.PayloadTerminalSetLayout](msg)
	if err != nil {
		return s.sendError(conn, err.Error())
	}
	s.terminals.SetLayout(payload.Session, payload.Blob, payload.Keep)
	return s.sendMsg(conn, &protocol.Msg{Type: protocol.MsgActionOK})
}

func (s *Server) handleTerminalListAll(conn net.Conn) bool {
	return s.sendMsg(conn, &protocol.Msg{
		Type:    protocol.MsgTerminalListAllOK,
		Payload: protocol.PayloadTerminalListAll{Terminals: s.terminals.ListAll()},
	})
}
