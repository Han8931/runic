// Package agent knows how to make a coding agent report itself to runic.
//
// The attention system is deliberately self-reported: a full-screen agent lives
// in the alt-screen, where guessing state from output is unreliable. But that
// only works if something actually calls `ru mark`, and until now that meant
// pasting a hook snippet out of the README by hand. A session that skipped the
// step looked identical to a healthy one — every state stayed empty, so the
// sort, the chips, the tab glyphs and the alarm all sat idle with nothing to
// report.
//
// So runic wires it up itself. When it starts an agent it knows the kind, and
// for kinds whose hooks it understands it instruments the launch command: the
// hooks are passed to the tool inline, writing nothing into the user's
// repository and leaving their own settings untouched.
package agent

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// hookCommands are the three transitions worth reporting, as shell commands run
// by the agent's own hook runner.
//
// `|| true` keeps the hook harmless when the agent is running outside a runic
// pane (no RUNIC_PANE, so `ru mark` has nothing to mark), and the 2>/dev/null
// keeps a stray error off the agent's own output. The note on `waiting` is the
// agent's question, which is the whole value of the row it lands on; jq is how
// the hook payload is read, and its absence degrades to an empty note rather
// than a lost mark.
// Stop reads its completion payload through ru itself, without a jq dependency.
var hookCommands = map[string]string{
	"UserPromptSubmit": `ru mark working 2>/dev/null || true`,
	"Notification":     `ru mark waiting "$(jq -r .message 2>/dev/null)" 2>/dev/null || true`,
	"Stop":             `ru mark done --claude-stop 2>/dev/null || true`,
}

// Claude Code's settings shape: each event holds matcher groups, each group a
// list of hooks. Only the command form is used here.
type claudeHook struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

type claudeHookGroup struct {
	Hooks []claudeHook `json:"hooks"`
}

type claudeSettings struct {
	Hooks map[string][]claudeHookGroup `json:"hooks"`
}

// ClaudeHookSettings is the settings document that makes Claude Code report its
// state to runic, as compact JSON.
func ClaudeHookSettings() string { return claudeHookJSON("") }

// ClaudeHookSettingsIndented is the same document formatted for a human to
// paste into .claude/settings.json (`ru agent hooks`).
func ClaudeHookSettingsIndented() string { return claudeHookJSON("  ") }

// claudeHookJSON encodes the settings document, indented when indent is set.
//
// HTML escaping is off: the hook commands contain `2>/dev/null`, and the
// default encoder would render that `2>/dev/null`. Both parse identically,
// but one of these is going to be read and pasted by a person.
func claudeHookJSON(indent string) string {
	s := claudeSettings{Hooks: make(map[string][]claudeHookGroup, len(hookCommands))}
	for event, cmd := range hookCommands {
		s.Hooks[event] = []claudeHookGroup{{Hooks: []claudeHook{{Type: "command", Command: cmd}}}}
	}
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if indent != "" {
		enc.SetIndent("", indent)
	}
	if err := enc.Encode(s); err != nil {
		// Unreachable: the value is plain strings, maps and slices.
		return ""
	}
	// Encode appends a newline, which would end up inside the shell-quoted
	// command on the launch path.
	return strings.TrimRight(buf.String(), "\n")
}

// Instrument adds self-reporting to an agent's launch command, returning the
// command to run and whether anything was added.
//
// It only ever touches a command it recognizes — the first word's base name has
// to be the tool whose settings flag it knows. A user who points "claude" at a
// wrapper script keeps their command verbatim, because appending a flag to an
// arbitrary program is a guess, and a launch that fails outright would be far
// worse than one that does not self-report. Everything after the first word is
// preserved, so `claude --permission-mode plan` stays instrumented.
func Instrument(cmd string) (string, bool) {
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return cmd, false
	}
	if filepath.Base(fields[0]) != "claude" {
		return cmd, false
	}
	// Already carrying settings of its own: leave it alone rather than pass the
	// flag twice and have one silently win.
	for _, f := range fields[1:] {
		if f == "--settings" || strings.HasPrefix(f, "--settings=") {
			return cmd, false
		}
	}
	// Single-quoted so the shell passes the JSON through untouched: the hook
	// bodies contain $( ) that must reach the agent literally, for *its* hook
	// runner to expand, not the launching shell. The JSON itself never contains
	// a single quote, so this cannot be broken out of.
	return cmd + " --settings '" + ClaudeHookSettings() + "'", true
}
