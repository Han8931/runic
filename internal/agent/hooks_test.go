package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

// The hook document has to be exactly what Claude Code expects, because a
// malformed one fails silently: the agent launches fine and simply never
// reports, which is indistinguishable from the feature not existing.
func TestClaudeHookSettingsShape(t *testing.T) {
	var doc struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(ClaudeHookSettings()), &doc); err != nil {
		t.Fatalf("settings are not valid JSON: %v", err)
	}
	for _, event := range []string{"UserPromptSubmit", "Notification", "Stop"} {
		groups, ok := doc.Hooks[event]
		if !ok || len(groups) != 1 || len(groups[0].Hooks) != 1 {
			t.Fatalf("event %s: want one group with one hook, got %+v", event, groups)
		}
		h := groups[0].Hooks[0]
		if h.Type != "command" {
			t.Errorf("event %s: type = %q, want command", event, h.Type)
		}
		if !strings.HasPrefix(h.Command, "ru mark ") {
			t.Errorf("event %s: command = %q, want a `ru mark`", event, h.Command)
		}
		// Running outside a runic pane must not fail the agent's hook.
		if !strings.HasSuffix(h.Command, "|| true") {
			t.Errorf("event %s: command %q must not be able to fail", event, h.Command)
		}
	}
}

// The three transitions have to map to the right states, or the dashboard
// reports the opposite of what is happening.
func TestClaudeHooksMapToStates(t *testing.T) {
	want := map[string]string{
		"UserPromptSubmit": "working",
		"Notification":     "waiting",
		"Stop":             "done",
	}
	for event, state := range want {
		cmd, ok := hookCommands[event]
		if !ok {
			t.Fatalf("no hook for %s", event)
		}
		if !strings.HasPrefix(cmd, "ru mark "+state) {
			t.Errorf("%s marks %q, want %s", event, cmd, state)
		}
	}
}

// The JSON is passed inside single quotes on a shell command line, so it must
// not contain one — and the $( ) in the hook body has to survive to the agent
// rather than being expanded by the launching shell.
func TestClaudeHookSettingsAreShellSafe(t *testing.T) {
	s := ClaudeHookSettings()
	if strings.Contains(s, "'") {
		t.Errorf("the settings JSON must not contain a single quote: %s", s)
	}
	if strings.Contains(s, "\n") {
		t.Error("the compact form must be one line, or it breaks the command")
	}
	if !strings.Contains(s, "$(") {
		t.Error("the waiting hook should still capture the agent's message")
	}
}

// Redirections must read as themselves for a human pasting the snippet.
func TestIndentedSettingsAreNotHTMLEscaped(t *testing.T) {
	s := ClaudeHookSettingsIndented()
	if strings.Contains(s, `\u003e`) {
		t.Errorf("redirections should not be HTML-escaped: %s", s)
	}
	if !strings.Contains(s, "2>/dev/null") {
		t.Errorf("expected a plain redirection in %s", s)
	}
	if !strings.Contains(s, "\n") {
		t.Error("the pasteable form should be indented")
	}
}

func TestInstrument(t *testing.T) {
	cases := []struct {
		name  string
		cmd   string
		want  bool
		check func(t *testing.T, got string)
	}{
		{
			name: "plain claude",
			cmd:  "claude",
			want: true,
			check: func(t *testing.T, got string) {
				if !strings.HasPrefix(got, "claude --settings '") || !strings.HasSuffix(got, "'") {
					t.Errorf("got %q", got)
				}
			},
		},
		{
			name: "claude with the user's own flags keeps them",
			cmd:  "claude --permission-mode plan",
			want: true,
			check: func(t *testing.T, got string) {
				if !strings.HasPrefix(got, "claude --permission-mode plan --settings '") {
					t.Errorf("the user's flags should survive: %q", got)
				}
			},
		},
		{
			name: "an absolute path to claude is still claude",
			cmd:  "/usr/local/bin/claude",
			want: true,
		},
		{
			// Appending a flag to an unknown program is a guess, and a launch
			// that fails outright is worse than one that does not self-report.
			name: "a wrapper script is left alone",
			cmd:  "~/bin/my-agent",
			want: false,
			check: func(t *testing.T, got string) {
				if got != "~/bin/my-agent" {
					t.Errorf("got %q, want the command verbatim", got)
				}
			},
		},
		{
			name: "another tool is left alone",
			cmd:  "codex",
			want: false,
		},
		{
			// Passing --settings twice would let one silently win.
			name: "an explicit --settings wins",
			cmd:  "claude --settings /my/own.json",
			want: false,
		},
		{
			name: "an explicit --settings= wins",
			cmd:  "claude --settings=/my/own.json",
			want: false,
		},
		{
			name: "empty stays empty",
			cmd:  "",
			want: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := Instrument(c.cmd)
			if ok != c.want {
				t.Fatalf("Instrument(%q) instrumented = %v, want %v", c.cmd, ok, c.want)
			}
			if !c.want && got != c.cmd {
				t.Errorf("an un-instrumented command must come back verbatim, got %q", got)
			}
			if c.check != nil {
				c.check(t, got)
			}
		})
	}
}

// Instrumenting twice must not stack flags — the second pass sees the first.
func TestInstrumentIsIdempotent(t *testing.T) {
	once, _ := Instrument("claude")
	twice, ok := Instrument(once)
	if ok {
		t.Error("an already-instrumented command should be left alone")
	}
	if twice != once {
		t.Errorf("second pass changed the command:\n%q\n%q", once, twice)
	}
}
