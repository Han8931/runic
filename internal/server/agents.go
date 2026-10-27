package server

import (
	"log"
	"strings"

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

// agentTailLines is the cap on a tail request, so a client cannot ask the
// daemon to render a megabyte of backlog into strings.
const agentTailLines = 40

// Tail returns the last n lines a pane printed, stripped of escape sequences.
//
// The daemon already keeps a backlog ring per pane to replay on attach, so this
// is a read of something it has rather than new capture. Cleanup happens here
// because a client that is not attached has no terminal emulator for the pane,
// and every client would otherwise repeat the same work.
//
// Caveat worth knowing: a full-screen program draws by moving the cursor, so its
// backlog is a sequence of partial repaints rather than a transcript. For those
// the tail is approximate. Agents that print progressively — which is the
// common case — read fine.
func (m *TerminalManager) Tail(session, pane string, n int) ([]string, bool) {
	if n <= 0 || n > agentTailLines {
		n = agentTailLines
	}
	key := terminalKey(session, pane)
	m.mu.Lock()
	t := m.sessions[key]
	m.mu.Unlock()
	if t == nil {
		return nil, false
	}
	t.mu.Lock()
	raw := t.backlog.bytes()
	t.mu.Unlock()
	return tailLines(raw, n), true
}

// tailLines turns raw pane output into the last n readable lines: escape
// sequences removed, carriage returns resolved, blank tail dropped.
func tailLines(raw []byte, n int) []string {
	text := stripANSI(string(raw))
	text = strings.ReplaceAll(text, "\r\n", "\n")
	// A bare CR rewrites the line it is on — progress bars and spinners lean on
	// it — so keep only what came after the last one.
	var kept []string
	for _, line := range strings.Split(text, "\n") {
		if i := strings.LastIndexByte(line, '\r'); i >= 0 {
			line = line[i+1:]
		}
		kept = append(kept, strings.TrimRight(line, " \t"))
	}
	// Trailing blanks are the prompt area, not output.
	for len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
		kept = kept[:len(kept)-1]
	}
	if len(kept) > n {
		kept = kept[len(kept)-n:]
	}
	// Leading blanks only pad the box they are shown in.
	for len(kept) > 0 && strings.TrimSpace(kept[0]) == "" {
		kept = kept[1:]
	}
	return kept
}

// stripANSI removes CSI and OSC sequences, which are control traffic rather
// than anything a person wants to read in a summary.
func stripANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] != 0x1b {
			if s[i] >= 0x20 || s[i] == '\n' || s[i] == '\r' || s[i] == '\t' {
				b.WriteByte(s[i])
			}
			i++
			continue
		}
		// ESC [ … final-byte   (CSI)
		if i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			i = j + 1
			continue
		}
		// ESC ] … BEL or ST    (OSC — the prompt markers live here)
		if i+1 < len(s) && s[i+1] == ']' {
			j := i + 2
			for j < len(s) && s[j] != 0x07 {
				if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
					j++
					break
				}
				j++
			}
			i = j + 1
			continue
		}
		// Any other two-byte escape.
		i += 2
	}
	return b.String()
}
