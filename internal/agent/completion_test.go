package agent

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestClaudeCompletion(t *testing.T) {
	for _, tc := range []struct{ name, input, want string }{
		{"final response", `{"last_assistant_message":"Fixed mouse navigation.\n\nTests passed.\tUpdated 3 files."}`, "Fixed mouse navigation. Tests passed. Updated 3 files."},
		{"literal shell text", "{\"last_assistant_message\":\"$(touch /tmp/never) `echo nope` --session other\"}", "$(touch /tmp/never) `echo nope` --session other"},
		{"terminal controls", `{"last_assistant_message":"\u001b[31mDone\u001b[0m\u0007"}`, "Done"},
		{"missing", `{}`, ""},
		{"null", `{"last_assistant_message":null}`, ""},
		{"wrong type", `{"last_assistant_message":42}`, ""},
		{"malformed", `{`, ""},
		{"empty input", "", ""},
		{"whitespace", `{"last_assistant_message":"\n\t "}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClaudeCompletion(strings.NewReader(tc.input)); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestClaudeCompletionBounds(t *testing.T) {
	input, _ := json.Marshal(map[string]string{"last_assistant_message": strings.Repeat("界", 2100)})
	got := ClaudeCompletion(strings.NewReader(string(input)))
	if !utf8.ValidString(got) || len([]rune(got)) != 2000 || !strings.HasSuffix(got, "…") {
		t.Fatal("preview must be bounded without splitting Unicode")
	}
	input, _ = json.Marshal(map[string]string{"last_assistant_message": strings.Repeat("x", 1<<20)})
	if got := ClaudeCompletion(strings.NewReader(string(input))); got != "" {
		t.Fatal("oversized hook payload should degrade to an empty note")
	}
}
