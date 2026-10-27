package agent

import (
	"encoding/json"
	"io"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// ClaudeCompletion reads the final response supplied by Claude's Stop hook.
// Missing or malformed payloads still allow the caller to report completion.
// Bound the stored note; the full response remains in the agent's session.
func ClaudeCompletion(input io.Reader) string {
	var payload struct {
		Message string `json:"last_assistant_message"`
	}
	if err := json.NewDecoder(io.LimitReader(input, 1<<20)).Decode(&payload); err != nil {
		return ""
	}
	text := strings.Join(strings.Fields(ansi.Strip(payload.Message)), " ")
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, text)
	runes := []rune(text)
	if len(runes) > 2000 {
		text = string(runes[:1999]) + "…"
	}
	return text
}
