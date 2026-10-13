//go:build windows

package tui

// queryDarkBackground is not implemented on Windows; lipgloss's own detection
// stays in effect, and the theme setting can force a palette.
func queryDarkBackground() (dark, ok bool) { return false, false }
