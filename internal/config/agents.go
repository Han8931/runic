package config

import (
	"sort"
	"strings"
)

// The agent registry maps a short kind — "claude", "codex" — to the command
// that starts it in a session pane. A session created with a kind gets that
// command typed into its first pane, so attaching lands on a running agent.
//
// Kinds are a small fixed vocabulary rather than free-form commands because the
// kind is also an identity: it is what the TUI labels the session with, and
// what makes a freshly created agent session visible before its first
// `ru mark`. The command behind a kind is configurable precisely because the
// built-in guesses will not suit everyone — a wrapper script, a model flag, or
// a tool runic has never heard of all belong in `agents`.

// DefaultAgents is the built-in kind → command table. The commands are the
// documented entry points of each tool, assumed to be on PATH; override them
// through the `agents` config key when yours differ.
var DefaultAgents = map[string]string{
	"claude": "claude",
	"codex":  "codex",
	"gemini": "gemini",
	"aider":  "aider",
}

// parseAgents reads the `agents` config value: comma-separated `kind=command`
// entries, e.g. `claude=claude --permission-mode plan, mine=~/bin/my-agent`.
//
// Entries merge over DefaultAgents rather than replacing the table, so naming
// one tool does not hide the rest. A kind mapped to an empty command removes it
// ("claude=" drops claude from the list), which is how you hide a tool you do
// not have installed.
func parseAgents(spec string) (map[string]string, []string) {
	agents := make(map[string]string, len(DefaultAgents))
	for kind, cmd := range DefaultAgents {
		agents[kind] = cmd
	}
	var warnings []string
	for _, entry := range strings.Split(spec, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		kind, cmd, ok := strings.Cut(entry, "=")
		kind = strings.TrimSpace(kind)
		if !ok || kind == "" {
			warnings = append(warnings, "agents: ignoring "+entry+" (want kind=command)")
			continue
		}
		if !validAgentKind(kind) {
			warnings = append(warnings,
				"agents: ignoring kind "+kind+" (letters, digits, - and _ only)")
			continue
		}
		if cmd = strings.TrimSpace(cmd); cmd == "" {
			delete(agents, kind)
			continue
		}
		agents[kind] = cmd
	}
	return agents, warnings
}

// validAgentKind keeps a kind usable as a label and as a CLI argument: it ends
// up in `ru session create --agent <kind>` and on a session row.
func validAgentKind(kind string) bool {
	for _, r := range kind {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_':
		default:
			return false
		}
	}
	return kind != ""
}

// AgentKinds lists the configured kinds in display order, which is the order
// the TUI's agent field cycles through and `ru session create --agent` accepts.
func (c *Config) AgentKinds() []string {
	kinds := make([]string, 0, len(c.Agents))
	for kind := range c.Agents {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	return kinds
}

// ResolveAgent looks a kind up in the registry, returning the registry's own
// spelling of it along with its command. Matching is case-insensitive so
// `--agent Claude` works, but the canonical spelling is what gets stored, so a
// session's kind reads the same however it was typed.
func (c *Config) ResolveAgent(kind string) (string, string, bool) {
	if kind = strings.TrimSpace(kind); kind == "" {
		return "", "", false
	}
	if cmd, ok := c.Agents[kind]; ok {
		return kind, cmd, true
	}
	for k, cmd := range c.Agents {
		if strings.EqualFold(k, kind) {
			return k, cmd, true
		}
	}
	return "", "", false
}
