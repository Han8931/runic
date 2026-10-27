package server

import (
	"log"

	"github.com/han/runic/internal/agent"
)

// Agent sessions: a session can name the kind of coding agent it hosts, and the
// daemon starts that agent in the session's first pane. The kind is stored on
// the queue (so it survives a daemon restart and is reported in SessionInfo),
// while the command behind it comes from the config registry — which means
// re-pointing "claude" at a wrapper script does not rewrite existing sessions.

// resolveAgent maps a requested kind to the registry's spelling of it and the
// command that starts it.
func (s *Server) resolveAgent(kind string) (string, string, bool) {
	return s.cfg.ResolveAgent(kind)
}

// sessionAgentCommand is the TerminalManager's lookup for "what should this
// session's first pane run?". An ordinary session answers "", and so does an
// agent session whose kind has since disappeared from the registry — the pane
// still opens, as a plain shell, because refusing to give someone a terminal
// over a config edit would be the wrong trade.
//
// The command comes back instrumented where runic knows how: an agent that
// cannot report itself leaves every attention state empty, which makes the
// whole dashboard — sort, chips, tab glyphs, alarm — silently inert. Wiring
// that up by hand was a documented step people reasonably skipped.
func (s *Server) sessionAgentCommand(session string) string {
	kind := s.jobs.SessionAgent(session)
	if kind == "" {
		return ""
	}
	_, cmd, ok := s.resolveAgent(kind)
	if !ok {
		return ""
	}
	cmd, instrumented := agent.Instrument(cmd)
	if !instrumented {
		log.Printf("session %s: starting %s without runic hooks — it will not "+
			"report its own state unless you wire `ru mark` yourself "+
			"(see `ru agent hooks`)", session, kind)
	}
	// Recorded now rather than when the pane dies, so the Agents view can tell
	// an agent that exited from one that was never opened.
	s.jobs.MarkAgentStarted(session)
	return cmd
}
