package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/han/runic/internal/config"
	"github.com/han/runic/internal/protocol"
)

func TestMarkSetsPaneAttention(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	m := NewTerminalManager()
	if _, err := m.GetOrCreate("s", "main", 80, 24); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	defer m.KillAll()

	if n := m.Mark("s", "main", protocol.AttentionWaiting, "needs review"); n != 1 {
		t.Fatalf("Mark touched %d panes, want 1", n)
	}
	got := m.SessionAttention()
	if got["s"].Attention != protocol.AttentionWaiting {
		t.Fatalf("session attention = %v, want waiting", got["s"].Attention)
	}
	if got["s"].AttentionNote != "needs review" {
		t.Fatalf("note = %q, want %q", got["s"].AttentionNote, "needs review")
	}
}

func TestMarkUnknownSessionTouchesNothing(t *testing.T) {
	m := NewTerminalManager()
	if n := m.Mark("nope", "", protocol.AttentionWaiting, ""); n != 0 {
		t.Fatalf("Mark on a session with no panes touched %d, want 0", n)
	}
}

func TestMarkEmptyPaneMarksWholeSession(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	m := NewTerminalManager()
	if _, err := m.GetOrCreate("s", "a", 80, 24); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	if _, err := m.GetOrCreate("s", "b", 80, 24); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	defer m.KillAll()

	if n := m.Mark("s", "", protocol.AttentionWorking, ""); n != 2 {
		t.Fatalf("Mark touched %d panes, want 2", n)
	}
}

// A session rolls up to its most urgent pane: one pane still working must not
// mask another that is blocked on the user.
func TestSessionAttentionTakesMostUrgentPane(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	m := NewTerminalManager()
	if _, err := m.GetOrCreate("s", "a", 80, 24); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	if _, err := m.GetOrCreate("s", "b", 80, 24); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	defer m.KillAll()

	m.Mark("s", "a", protocol.AttentionWorking, "")
	m.Mark("s", "b", protocol.AttentionWaiting, "")
	if got := m.SessionAttention()["s"].Attention; got != protocol.AttentionWaiting {
		t.Fatalf("rollup = %v, want waiting", got)
	}

	// ...and an error does not outrank a pane blocked on input.
	m.Mark("s", "a", protocol.AttentionError, "")
	if got := m.SessionAttention()["s"].Attention; got != protocol.AttentionWaiting {
		t.Fatalf("rollup = %v, want waiting to still win", got)
	}
}

func TestAttentionHookFiresOnlyOnTransition(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	m := NewTerminalManager()
	events := make(chan AttentionEvent, 8)
	m.SetAttentionHook(func(ev AttentionEvent) { events <- ev })
	if _, err := m.GetOrCreate("s", "main", 80, 24); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	defer m.KillAll()

	m.Mark("s", "main", protocol.AttentionWorking, "")
	select {
	case ev := <-events:
		if ev.From != protocol.AttentionNone || ev.To != protocol.AttentionWorking {
			t.Fatalf("event = %v -> %v, want none -> working", ev.From, ev.To)
		}
	case <-time.After(time.Second):
		t.Fatal("no event for the first mark")
	}

	// Re-reporting the same state is not a transition — a hook that polls
	// `ru mark working` in a loop must not notify on every call.
	m.Mark("s", "main", protocol.AttentionWorking, "")
	select {
	case ev := <-events:
		t.Fatalf("unexpected event for an unchanged state: %v -> %v", ev.From, ev.To)
	case <-time.After(150 * time.Millisecond):
	}
}

// A shell prompt proves nothing is running, so a stale "working" mark clears
// itself — an agent that died before reporting does not show busy forever.
func TestPromptMarkerClearsWorking(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	m := NewTerminalManager()
	term, err := m.GetOrCreate("s", "main", 80, 24)
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	defer m.KillAll()

	m.Mark("s", "main", protocol.AttentionWorking, "building")
	term.broadcast([]byte("some output" + promptMarker))
	if got, _, _ := term.Attention(); got != protocol.AttentionIdle {
		t.Fatalf("attention = %v after a prompt, want idle", got)
	}
}

// ...but it must not clear "waiting" or "error": an agent's finish hook fires
// just before its process exits, so the prompt right after would wipe exactly
// the state the user needs to see.
func TestPromptMarkerKeepsNeedsYouStates(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	m := NewTerminalManager()
	term, err := m.GetOrCreate("s", "main", 80, 24)
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	defer m.KillAll()

	for _, want := range []protocol.Attention{protocol.AttentionWaiting, protocol.AttentionError} {
		m.Mark("s", "main", want, "note")
		term.broadcast([]byte(promptMarker))
		if got, _, _ := term.Attention(); got != want {
			t.Fatalf("attention = %v after a prompt, want %v preserved", got, want)
		}
	}
}

// Attaching to a pane is how the alert is acknowledged, so a "needs you" state
// clears; a pane still working keeps its state (watching it is not finishing it).
func TestSeenClearsOnlyNeedsYou(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	m := NewTerminalManager()
	term, err := m.GetOrCreate("s", "main", 80, 24)
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	defer m.KillAll()

	m.Mark("s", "main", protocol.AttentionWaiting, "answer me")
	term.fire(term.seen())
	if got, note, _ := term.Attention(); got != protocol.AttentionIdle || note != "" {
		t.Fatalf("attention = %v/%q after attach, want idle and no note", got, note)
	}

	m.Mark("s", "main", protocol.AttentionWorking, "busy")
	term.fire(term.seen())
	if got, _, _ := term.Attention(); got != protocol.AttentionWorking {
		t.Fatalf("attention = %v after attach, want working preserved", got)
	}
}

func TestPaneEnvCarriesSessionAndPane(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	m := NewTerminalManager()
	term, err := m.GetOrCreate("envtest", "p1", 80, 24)
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	defer m.KillAll()

	var sawSession, sawPane bool
	for _, kv := range term.cmd.Env {
		switch kv {
		case "RUNIC_SESSION=envtest":
			sawSession = true
		case "RUNIC_PANE=p1":
			sawPane = true
		}
	}
	if !sawSession || !sawPane {
		t.Fatalf("pane env missing RUNIC_SESSION/RUNIC_PANE: %v", term.cmd.Env)
	}
}

// The on_attention hook must actually run, with the transition in its
// environment, so a desktop notification or bell can be wired up from config.
func TestOnAttentionHookRuns(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	dir := t.TempDir()
	stamp := filepath.Join(dir, "fired")

	srv := &Server{
		cfg:       &config.Config{OnAttention: "printf '%s %s %s' \"$RUNIC_ATTENTION_FROM\" \"$RUNIC_ATTENTION\" \"$RUNIC_NOTE\" > " + stamp},
		terminals: NewTerminalManager(),
	}
	srv.terminals.SetAttentionHook(srv.onAttentionChanged)
	if _, err := srv.terminals.GetOrCreate("s", "main", 80, 24); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	defer srv.terminals.KillAll()

	srv.terminals.Mark("s", "main", protocol.AttentionWaiting, "pick one")

	var data []byte
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(stamp)
		if err == nil && len(b) > 0 {
			data = b
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got, want := string(data), "none waiting pick one"; got != want {
		t.Fatalf("hook environment = %q, want %q", got, want)
	}
}
