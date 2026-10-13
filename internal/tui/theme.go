package tui

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// applyTheme picks which half of every AdaptiveColor in styles.go renders. It
// must run before the program starts: lipgloss resolves the background once
// and caches it, and the auto query reads the terminal's reply off the tty,
// which Bubble Tea would otherwise consume as keystrokes.
//
// "auto" asks the terminal itself rather than leaving it to lipgloss, because
// termenv refuses to query under tmux and screen and then assumes a black
// background — so on a light terminal inside tmux every token would resolve to
// its dark variant. tmux answers the query fine on its own. If the terminal
// does not answer, lipgloss's own detection is left in place.
func applyTheme(mode string) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "light":
		lipgloss.SetHasDarkBackground(false)
	case "dark":
		lipgloss.SetHasDarkBackground(true)
	default:
		if dark, ok := queryDarkBackground(); ok {
			lipgloss.SetHasDarkBackground(dark)
		}
	}
}

// osc11Reply matches a terminal's answer to the OSC 11 background query,
// e.g. "\x1b]11;rgb:ffff/fafa/f0f0\x07". Some terminals answer "rgba:" with a
// trailing alpha component, which is ignored.
var osc11Reply = regexp.MustCompile(`(?i)\x1b\]11;rgba?:([0-9a-f]{1,4})/([0-9a-f]{1,4})/([0-9a-f]{1,4})`)

// da1Reply matches the primary device attributes answer that follows the OSC
// query; every terminal answers it, so its arrival means no OSC reply is coming.
var da1Reply = regexp.MustCompile(`\x1b\[\?[0-9;]*c`)

// parseBackgroundReply extracts whether the background in an OSC 11 reply is
// dark, using the same HSL-lightness cutoff as termenv. ok is false when the
// buffer holds no reply.
func parseBackgroundReply(buf string) (dark, ok bool) {
	m := osc11Reply.FindStringSubmatch(buf)
	if m == nil {
		return false, false
	}
	var ch [3]float64
	for i, hex := range m[1:] {
		v, err := strconv.ParseUint(hex, 16, 16)
		if err != nil {
			return false, false
		}
		// Each component is scaled to its own digit count: "f" and "ffff"
		// both mean full intensity.
		ch[i] = float64(v) / float64(uint64(1)<<(4*len(hex))-1)
	}
	hi := max(ch[0], ch[1], ch[2])
	lo := min(ch[0], ch[1], ch[2])
	return (hi+lo)/2 < 0.5, true
}
