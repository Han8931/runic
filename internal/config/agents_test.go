package config

import (
	"strings"
	"testing"
)

func TestParseAgentsMergesOverDefaults(t *testing.T) {
	agents, warnings := parseAgents("claude=claude --permission-mode plan, mine=~/bin/my-agent")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	// The named entry wins...
	if got := agents["claude"]; got != "claude --permission-mode plan" {
		t.Errorf("claude = %q, want the configured command", got)
	}
	// ...a new kind is added...
	if got := agents["mine"]; got != "~/bin/my-agent" {
		t.Errorf("mine = %q, want the configured command", got)
	}
	// ...and naming one tool does not hide the others.
	for _, kind := range []string{"codex", "gemini", "aider"} {
		if agents[kind] != DefaultAgents[kind] {
			t.Errorf("%s was dropped by an unrelated override", kind)
		}
	}
}

// An empty command removes a kind, which is how you hide a tool you do not have.
func TestParseAgentsEmptyCommandRemoves(t *testing.T) {
	agents, _ := parseAgents("aider=")
	if _, ok := agents["aider"]; ok {
		t.Error("aider= should remove the kind from the registry")
	}
	if agents["claude"] != "claude" {
		t.Error("removing one kind must not disturb the rest")
	}
}

func TestParseAgentsRejectsMalformedEntries(t *testing.T) {
	agents, warnings := parseAgents("notakeypair, =orphan, bad kind=x, ok=run-it")
	if len(warnings) != 3 {
		t.Errorf("got %d warnings, want one per bad entry: %v", len(warnings), warnings)
	}
	if agents["ok"] != "run-it" {
		t.Error("a valid entry alongside bad ones should still land")
	}
	for _, w := range warnings {
		if !strings.HasPrefix(w, "agents:") {
			t.Errorf("warning %q should name the setting it came from", w)
		}
	}
	// A rejected kind must not be silently half-registered.
	if _, ok := agents["bad kind"]; ok {
		t.Error("a kind with a space should be refused, not stored")
	}
}

func TestParseAgentsEmptySpecIsDefaults(t *testing.T) {
	agents, warnings := parseAgents("")
	if len(warnings) != 0 {
		t.Errorf("an empty spec is not an error: %v", warnings)
	}
	if len(agents) != len(DefaultAgents) {
		t.Errorf("got %d kinds, want the %d built-ins", len(agents), len(DefaultAgents))
	}
}

func TestResolveAgentCanonicalizesSpelling(t *testing.T) {
	c := &Config{Agents: map[string]string{"claude": "claude"}}
	// Typed any way, the registry's own spelling is what gets stored, so a
	// session's kind reads consistently however it was created.
	for _, typed := range []string{"claude", "Claude", "CLAUDE", "  claude  "} {
		kind, cmd, ok := c.ResolveAgent(typed)
		if !ok || kind != "claude" || cmd != "claude" {
			t.Errorf("ResolveAgent(%q) = (%q, %q, %v)", typed, kind, cmd, ok)
		}
	}
	if _, _, ok := c.ResolveAgent("codex"); ok {
		t.Error("a kind outside the registry must not resolve")
	}
	if _, _, ok := c.ResolveAgent(""); ok {
		t.Error("an empty kind is not an agent")
	}
}

func TestAgentKindsSorted(t *testing.T) {
	c := &Config{Agents: map[string]string{"zeta": "z", "alpha": "a", "mid": "m"}}
	if got := strings.Join(c.AgentKinds(), ","); got != "alpha,mid,zeta" {
		t.Errorf("AgentKinds = %q, want them sorted", got)
	}
}

// The registry reaches Config through the normal layered load, so `agents` in
// the config file is actually in effect.
func TestLoadPopulatesAgentRegistry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("RUNIC_AGENTS", "codex=codex --full-auto")
	cfg := Load()
	if got := cfg.Agents["codex"]; got != "codex --full-auto" {
		t.Errorf("codex = %q, want the value from the environment", got)
	}
	if cfg.Agents["claude"] != "claude" {
		t.Error("the built-ins should survive an environment override")
	}
}
