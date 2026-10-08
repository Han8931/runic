package server

import (
	"bytes"
	"log"
	"net"
	"os"
	"os/exec"
	"time"

	"github.com/han/runic/internal/protocol"
)

// Per-pane attention state: the "is this agent working, stuck, or waiting on
// me" signal that turns the session list into an inbox. Panes report it
// themselves with `ru mark` (they get RUNIC_SESSION and RUNIC_PANE in their
// environment), because a full-screen agent in the alt-screen gives the daemon
// nothing reliable to infer it from.

// AttentionEvent describes one attention transition, handed to the manager's
// hook so the daemon can notify on it.
type AttentionEvent struct {
	Session string
	Pane    string
	From    protocol.Attention
	To      protocol.Attention
	Note    string
}

// SetAttentionHook installs the callback fired on every attention transition.
// It runs on the goroutine that caused the change (a request handler or a
// pane's read loop) with no lock held, so it must not block for long.
func (m *TerminalManager) SetAttentionHook(fn func(AttentionEvent)) {
	m.mu.Lock()
	m.onAttention = fn
	for _, t := range m.sessions {
		t.setHook(fn)
	}
	m.mu.Unlock()
}

// Mark sets the attention state of one pane, or of every live pane in the
// session when pane is empty. It returns how many panes it touched; zero means
// the session has no live panes, which the caller reports as an error rather
// than silently accepting a mark nothing will ever show.
//
// A named pane that is not in the named session falls back to a match on the
// pane name alone, and only when it is unambiguous. That is what lets a shell
// started before a session rename keep reporting: its RUNIC_SESSION is the old
// name, but RUNIC_PANE is the pane's permanent identity.
func (m *TerminalManager) Mark(session, pane string, a protocol.Attention, note string) int {
	if session == "" {
		session = "default"
	}
	m.mu.Lock()
	var targets []*TerminalPTY
	for _, t := range m.sessions {
		if t.sessionName() != session || t.isDone() {
			continue
		}
		if pane != "" && t.pane != pane {
			continue
		}
		targets = append(targets, t)
	}
	if len(targets) == 0 && pane != "" {
		for _, t := range m.sessions {
			if t.pane == pane && !t.isDone() {
				targets = append(targets, t)
			}
		}
		if len(targets) > 1 {
			// Two sessions each have a pane by that name (the implicit "main"),
			// so the name alone does not identify one. Refuse rather than mark
			// the wrong agent's pane.
			targets = nil
		}
	}
	m.mu.Unlock()

	now := time.Now()
	for _, t := range targets {
		t.fire(t.setAttention(a, note, now))
	}
	return len(targets)
}

// SessionAttention rolls every live pane up to its session: a session's state is
// the most urgent of its panes'. Attention's constants are ordered by urgency,
// so this is a plain max.
func (m *TerminalManager) SessionAttention() map[string]protocol.SessionInfo {
	m.mu.Lock()
	terms := make([]*TerminalPTY, 0, len(m.sessions))
	for _, t := range m.sessions {
		terms = append(terms, t)
	}
	m.mu.Unlock()

	out := make(map[string]protocol.SessionInfo, len(terms))
	for _, t := range terms {
		if t.isDone() {
			continue
		}
		a, note, since := t.Attention()
		if a == protocol.AttentionNone {
			continue
		}
		name := t.sessionName()
		// Equal urgency keeps the pane that has been in the state longest: with
		// two agents waiting, the one you have kept waiting is the one the
		// session should be reporting.
		if cur, ok := out[name]; ok {
			if cur.Attention > a {
				continue
			}
			if cur.Attention == a && !cur.AttentionSince.IsZero() && !cur.AttentionSince.After(since) {
				continue
			}
		}
		out[name] = protocol.SessionInfo{Attention: a, AttentionNote: note, AttentionSince: since}
	}
	return out
}

// Attention reports the pane's current self-reported state, the note that came
// with it, and when it was set.
func (t *TerminalPTY) Attention() (protocol.Attention, string, time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.attention, t.note, t.since
}

// setAttention records a new state and returns the transition to fire, or nil
// when nothing changed (so a hook that polls `ru mark working` in a loop does
// not notify on every call).
func (t *TerminalPTY) setAttention(a protocol.Attention, note string, now time.Time) *AttentionEvent {
	t.mu.Lock()
	prev, prevNote := t.attention, t.note
	t.attention, t.note, t.since = a, note, now
	// The session label can be reassigned by a rename; read it here, under the
	// same lock that guards it, so the event names the session the pane is in.
	session := t.session
	t.mu.Unlock()

	if prev == a && prevNote == note {
		return nil
	}
	return &AttentionEvent{Session: session, Pane: t.pane, From: prev, To: a, Note: note}
}

// seen clears a "needs you" state once a client actually attaches to the pane:
// opening the session is how you acknowledge its alert, so the inbox empties as
// you work through it. A pane still claiming to be working keeps that state —
// attaching to watch it does not make it idle.
func (t *TerminalPTY) seen() *AttentionEvent {
	t.mu.Lock()
	if !t.attention.NeedsYou() {
		t.mu.Unlock()
		return nil
	}
	t.mu.Unlock()
	return t.setAttention(protocol.AttentionIdle, "", time.Now())
}

// noteOutputLocked inspects a chunk of pane output for the shell's OSC 133
// prompt marker. A prompt means whatever was running has exited, which
// disproves a "working" claim — so a stale mark left behind by an agent that
// crashed before reporting clears itself instead of showing busy forever.
//
// It deliberately does NOT clear "waiting" or "error": an agent's finish hook
// fires just before its process exits, so the prompt that follows would wipe
// exactly the state you most need to see.
//
// Caller must hold t.mu.
func (t *TerminalPTY) noteOutputLocked(data []byte) *AttentionEvent {
	if t.attention != protocol.AttentionWorking || !bytes.Contains(data, []byte(promptMarker)) {
		return nil
	}
	prev := t.attention
	t.attention, t.note, t.since = protocol.AttentionIdle, "", time.Now()
	return &AttentionEvent{Session: t.session, Pane: t.pane, From: prev, To: protocol.AttentionIdle}
}

// fire hands a transition to the manager's hook. Safe with a nil event.
func (t *TerminalPTY) fire(ev *AttentionEvent) {
	if ev == nil {
		return
	}
	t.mu.Lock()
	hook := t.onAttention
	t.mu.Unlock()
	if hook != nil {
		hook(*ev)
	}
}

func (t *TerminalPTY) setHook(fn func(AttentionEvent)) {
	t.mu.Lock()
	t.onAttention = fn
	t.mu.Unlock()
}

// onAttentionChanged is the daemon's attention hook: it logs every transition
// and runs the `on_attention` command, which is how a desktop notification or a
// terminal bell gets fired. It is called from a request handler or a pane's read
// loop, so it must never block — the hook process is started and reaped in the
// background, exactly like on_finish.
func (s *Server) onAttentionChanged(ev AttentionEvent) {
	log.Printf("pane %s/%s attention: %s -> %s", ev.Session, ev.Pane, ev.From, ev.To)

	hook := s.cfg.OnAttention
	if hook == "" {
		return
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.Command(shell, "-c", hook)
	// Every transition fires, including quieting ones (working -> idle). The
	// hook filters on RUNIC_ATTENTION; RUNIC_ATTENTION_FROM says what it was,
	// so "only tell me when it starts needing me" is a one-line test.
	cmd.Env = append(os.Environ(),
		"RUNIC_SESSION="+ev.Session,
		"RUNIC_PANE="+ev.Pane,
		"RUNIC_ATTENTION="+ev.To.String(),
		"RUNIC_ATTENTION_FROM="+ev.From.String(),
		"RUNIC_NOTE="+ev.Note,
	)
	if err := cmd.Start(); err != nil {
		log.Printf("on_attention hook: %v", err)
		return
	}
	go func() { _ = cmd.Wait() }()
}

func (s *Server) handleMark(conn net.Conn, msg *protocol.Msg) bool {
	payload, err := protocol.PayloadAs[protocol.PayloadMark](msg)
	if err != nil {
		return s.sendError(conn, err.Error())
	}
	n := s.terminals.Mark(payload.Session, payload.Pane, payload.Attention, payload.Note)
	if n == 0 {
		where := "session " + payload.Session
		if payload.Pane != "" {
			where += " pane " + payload.Pane
		}
		return s.sendError(conn, "no live pane in "+where)
	}
	return s.sendMsg(conn, &protocol.Msg{Type: protocol.MsgActionOK})
}
