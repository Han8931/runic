package server_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/han/runic/internal/client"
	"github.com/han/runic/internal/protocol"
)

// Agent sessions end to end: the kind is recorded on the session, and the
// daemon actually starts that agent in the session's first pane.
//
// The "agent" is a script that appends a line to a file, so the test can tell
// not just that it ran but how many times — which is the behaviour that matters
// when a session gets a second pane.

// agentProbe writes a script that records each run, and registers it as an
// agent kind. Returns the kind and the path the runs are recorded to.
func agentProbe(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "runs.log")
	script := filepath.Join(dir, "probe-agent")
	body := "#!/bin/sh\necho started >> " + log + "\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatalf("write probe agent: %v", err)
	}
	t.Setenv("RUNIC_AGENTS", "probe="+script)
	return "probe", log
}

// probeRuns counts how many times the probe agent has run so far.
func probeRuns(t *testing.T, log string) int {
	t.Helper()
	data, err := os.ReadFile(log)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatalf("read probe log: %v", err)
	}
	return len(strings.Fields(strings.TrimSpace(string(data))))
}

// waitForProbeRuns waits for the probe to have run at least n times.
func waitForProbeRuns(t *testing.T, log string, n int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if probeRuns(t, log) >= n {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("probe agent ran %d times, want %d", probeRuns(t, log), n)
}

func TestAgentSessionStartsAgentInFirstPane(t *testing.T) {
	kind, log := agentProbe(t)
	startDaemon(t)

	if err := client.SessionCreateSpec(client.SessionSpec{Name: "a1", Agent: kind}); err != nil {
		t.Fatalf("SessionCreateSpec: %v", err)
	}

	// The kind reaches the TUI through the tree poll, before any pane exists:
	// that is what makes a fresh agent session visible immediately rather than
	// only after the tool's first `ru mark`.
	data, err := client.TreeData()
	if err != nil {
		t.Fatalf("TreeData: %v", err)
	}
	var found bool
	for _, s := range data.Sessions {
		if s.Name == "a1" {
			found = true
			if s.Agent != kind {
				t.Errorf("session agent = %q, want %q", s.Agent, kind)
			}
		}
	}
	if !found {
		t.Fatalf("session a1 missing from the tree: %+v", data.Sessions)
	}

	pane, err := client.OpenTerminal("a1", 80, 24)
	if err != nil {
		t.Fatalf("OpenTerminal: %v", err)
	}
	waitForProbeRuns(t, log, 1)

	// Launching an agent is a fact, so the pane reports working without the
	// agent having to say so itself.
	terms, err := client.ListTerminals()
	if err != nil {
		t.Fatalf("ListTerminals: %v", err)
	}
	for _, term := range terms {
		if term.Session == "a1" && term.Pane == pane {
			if term.Attention != protocol.AttentionWorking {
				t.Errorf("pane attention = %v, want working", term.Attention)
			}
			if !strings.Contains(term.Note, "probe-agent") {
				t.Errorf("pane note = %q, want it to name what was started", term.Note)
			}
		}
	}
}

// The whole reason the attention system looked broken: an agent that is never
// told to report stays silent, and a silent agent is indistinguishable from a
// missing feature. A session runic starts must arrive already instrumented.
//
// The probe is named `claude` so it takes the instrumented path, and records
// its own argv — so this asserts the hooks survive config lookup, registry
// resolution, instrumentation, the generated rc, and the shell's own quoting,
// which is the part most likely to mangle a JSON argument.
func TestAgentSessionLaunchesClaudeWithHooks(t *testing.T) {
	dir := t.TempDir()
	argvLog := filepath.Join(dir, "argv")
	script := filepath.Join(dir, "claude")
	// "$@" with one argument per line: the point is to prove the JSON arrived
	// as a *single* argument rather than split on its spaces.
	body := "#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\"; done > " + argvLog + "\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatalf("write probe: %v", err)
	}
	t.Setenv("RUNIC_AGENTS", "claude="+script)
	startDaemon(t)

	if err := client.SessionCreateSpec(client.SessionSpec{Name: "a1", Agent: "claude"}); err != nil {
		t.Fatalf("SessionCreateSpec: %v", err)
	}
	if _, err := client.OpenTerminal("a1", 80, 24); err != nil {
		t.Fatalf("OpenTerminal: %v", err)
	}

	var argv []string
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(argvLog)
		if err == nil && len(data) > 0 {
			argv = strings.Split(strings.TrimRight(string(data), "\n"), "\n")
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if len(argv) != 2 {
		t.Fatalf("claude got %d args %q, want exactly --settings and one JSON blob", len(argv), argv)
	}
	if argv[0] != "--settings" {
		t.Errorf("first arg = %q, want --settings", argv[0])
	}
	// The JSON must have survived the shell intact, hooks and all.
	var doc struct {
		Hooks map[string][]struct {
			Hooks []struct{ Command string } `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(argv[1]), &doc); err != nil {
		t.Fatalf("the settings argument is not valid JSON after the shell saw it: %v\n%s", err, argv[1])
	}
	for _, event := range []string{"UserPromptSubmit", "Notification", "Stop"} {
		groups, ok := doc.Hooks[event]
		if !ok || len(groups) == 0 || len(groups[0].Hooks) == 0 {
			t.Fatalf("event %s missing from the delivered settings: %s", event, argv[1])
		}
		if !strings.Contains(groups[0].Hooks[0].Command, "ru mark") {
			t.Errorf("event %s command = %q, want a `ru mark`", event, groups[0].Hooks[0].Command)
		}
	}
}

// The rc path, which is what bash and zsh actually use: the agent is launched
// from the generated rc after the user's own, rather than typed into the pane.
// The sh test above covers the type-ahead fallback.
func TestAgentSessionStartsAgentFromShellRC(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not installed")
	}
	kind, log := agentProbe(t)
	startDaemon(t)
	// Applies to panes opened from here on: the shell is read at spawn time.
	t.Setenv("SHELL", bash)

	if err := client.SessionCreateSpec(client.SessionSpec{Name: "a1", Agent: kind}); err != nil {
		t.Fatalf("SessionCreateSpec: %v", err)
	}
	if _, err := client.OpenTerminal("a1", 80, 24); err != nil {
		t.Fatalf("OpenTerminal: %v", err)
	}
	waitForProbeRuns(t, log, 1)

	// A shell started *inside* the pane re-sources the same rc through its
	// environment, which must not start a second agent.
	time.Sleep(500 * time.Millisecond)
	if runs := probeRuns(t, log); runs != 1 {
		t.Errorf("probe ran %d times, want exactly 1", runs)
	}
}

// Splitting an agent session gives you a shell next to the agent, not a second
// copy of it.
func TestAgentSessionDoesNotRelaunchInLaterPanes(t *testing.T) {
	kind, log := agentProbe(t)
	startDaemon(t)

	if err := client.SessionCreateSpec(client.SessionSpec{Name: "a1", Agent: kind}); err != nil {
		t.Fatalf("SessionCreateSpec: %v", err)
	}
	if _, err := client.OpenTerminal("a1", 80, 24); err != nil {
		t.Fatalf("OpenTerminal: %v", err)
	}
	waitForProbeRuns(t, log, 1)

	if _, err := client.OpenTerminal("a1", 80, 24); err != nil {
		t.Fatalf("OpenTerminal (second pane): %v", err)
	}
	// Give a second launch time to show up if it were going to.
	time.Sleep(500 * time.Millisecond)
	if runs := probeRuns(t, log); runs != 1 {
		t.Errorf("probe ran %d times, want exactly 1 (the session's first pane)", runs)
	}
}

// A plain session's panes are plain shells: no agent, no spurious working state.
func TestPlainSessionStartsNoAgent(t *testing.T) {
	_, log := agentProbe(t)
	startDaemon(t)

	if err := client.SessionCreate("plain"); err != nil {
		t.Fatalf("SessionCreate: %v", err)
	}
	pane, err := client.OpenTerminal("plain", 80, 24)
	if err != nil {
		t.Fatalf("OpenTerminal: %v", err)
	}
	time.Sleep(500 * time.Millisecond)
	if runs := probeRuns(t, log); runs != 0 {
		t.Errorf("probe ran %d times for a session that asked for no agent", runs)
	}
	terms, err := client.ListTerminals()
	if err != nil {
		t.Fatalf("ListTerminals: %v", err)
	}
	for _, term := range terms {
		if term.Session == "plain" && term.Pane == pane && term.Attention == protocol.AttentionWorking {
			t.Error("a plain shell should not report working on its own")
		}
	}
}

// An unknown kind fails the create outright. A session that quietly turned out
// to be a plain shell would be worse: you would only find out by attaching.
func TestAgentSessionUnknownKindRefused(t *testing.T) {
	agentProbe(t)
	startDaemon(t)

	err := client.SessionCreateSpec(client.SessionSpec{Name: "a1", Agent: "nope"})
	if err == nil {
		t.Fatal("expected an error for an unknown agent kind")
	}
	if !strings.Contains(err.Error(), "probe") {
		t.Errorf("error %q should list the kinds that are known", err)
	}
	// And nothing is left behind: the validation happens before the create.
	sessions, lErr := client.SessionList()
	if lErr != nil {
		t.Fatalf("SessionList: %v", lErr)
	}
	for _, s := range sessions {
		if s == "a1" {
			t.Error("a refused agent session must not be created anyway")
		}
	}
}

// The kind survives a rename, like the worktree does — attention roll-up and
// the agent label have to keep pointing at one session.
func TestAgentSessionSurvivesRename(t *testing.T) {
	kind, _ := agentProbe(t)
	startDaemon(t)

	if err := client.SessionCreateSpec(client.SessionSpec{Name: "a1", Agent: kind}); err != nil {
		t.Fatalf("SessionCreateSpec: %v", err)
	}
	if err := client.SessionRename("a1", "a2"); err != nil {
		t.Fatalf("SessionRename: %v", err)
	}
	data, err := client.TreeData()
	if err != nil {
		t.Fatalf("TreeData: %v", err)
	}
	for _, s := range data.Sessions {
		if s.Name == "a2" {
			if s.Agent != kind {
				t.Errorf("after rename agent = %q, want %q", s.Agent, kind)
			}
			return
		}
	}
	t.Fatal("renamed session missing from the tree")
}

// The snippet, end to end: what a pane printed reaches a client that is not
// attached to it. "done" on its own answers nothing — the Stop hook fires as
// the agent exits and carries no message — so the only way to find out what an
// agent did was to attach and scroll.
func TestPaneTailOverTheWire(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "talkative")
	body := "#!/bin/sh\nprintf 'first\\nsecond\\nthird\\n'\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatalf("write probe: %v", err)
	}
	t.Setenv("RUNIC_AGENTS", "talker="+script)
	startDaemon(t)

	if err := client.SessionCreateSpec(client.SessionSpec{Name: "s", Agent: "talker"}); err != nil {
		t.Fatalf("SessionCreateSpec: %v", err)
	}
	pane, err := client.OpenTerminal("s", 80, 24)
	if err != nil {
		t.Fatalf("OpenTerminal: %v", err)
	}

	var lines []string
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		lines, err = client.TerminalTail("s", pane, 5)
		if err == nil && strings.Contains(strings.Join(lines, "\n"), "third") {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("TerminalTail: %v", err)
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"first", "second", "third"} {
		if !strings.Contains(joined, want) {
			t.Errorf("tail missing %q:\n%s", want, joined)
		}
	}
	// No escape sequences survive: the shell's prompt markers and any color are
	// control traffic, not something to show in a summary.
	if strings.Contains(joined, "\x1b") {
		t.Errorf("tail still carries escape sequences: %q", joined)
	}

	if _, err := client.TerminalTail("s", "ghost", 5); err == nil {
		t.Error("a pane that does not exist should be an error, not empty output")
	}
}

// Acknowledging over the wire. This path was unregistered with gob when it was
// written, so it would have failed the moment anyone pressed the key — the
// encode error only shows up at runtime.
func TestAckOverTheWire(t *testing.T) {
	startDaemon(t)
	if err := client.SessionCreate("s"); err != nil {
		t.Fatalf("SessionCreate: %v", err)
	}
	pane, err := client.OpenTerminal("s", 80, 24)
	if err != nil {
		t.Fatalf("OpenTerminal: %v", err)
	}
	if err := client.Mark("s", pane, protocol.AttentionWaiting, "which schema?"); err != nil {
		t.Fatalf("Mark: %v", err)
	}

	if err := client.Ack("s", pane); err != nil {
		t.Fatalf("Ack: %v", err)
	}

	terms, err := client.ListTerminals()
	if err != nil {
		t.Fatalf("ListTerminals: %v", err)
	}
	for _, term := range terms {
		if term.Session != "s" || term.Pane != pane {
			continue
		}
		// Acknowledged, not answered: the question is still the most useful
		// thing on the row.
		if term.Attention != protocol.AttentionWaiting {
			t.Errorf("attention = %v, want waiting preserved", term.Attention)
		}
		if term.Note != "which schema?" {
			t.Errorf("note = %q, want it kept", term.Note)
		}
		if !term.Seen {
			t.Error("the alert should be marked seen")
		}
		if protocol.Unresolved(term.Attention, term.Seen) {
			t.Error("an acknowledged pane should stop counting as needing you")
		}
		return
	}
	t.Fatal("pane missing from ListTerminals")
}
