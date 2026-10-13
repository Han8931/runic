//go:build !windows

package tui

import (
	"os"
	"time"

	"github.com/charmbracelet/x/term"
	"golang.org/x/sys/unix"
)

// themeQueryTimeout bounds startup on a terminal that answers neither query.
// Real terminals answer the device-attributes sentinel within milliseconds;
// the headroom is for slow ssh links, where a reply that misses the deadline
// is both a wrong guess and stray input for the TUI.
const themeQueryTimeout = 500 * time.Millisecond

// queryDarkBackground asks the controlling terminal for its background color
// (OSC 11), followed by a primary device attributes request (DA1) as a
// sentinel: terminals answer in order, so reading up to the DA1 reply drains
// the OSC reply too, and a terminal that ignores OSC 11 is detected without
// waiting out the timeout.
func queryDarkBackground() (dark, ok bool) {
	// A dumb terminal answers neither query; don't make it wait the timeout.
	if os.Getenv("TERM") == "dumb" {
		return false, false
	}
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return false, false
	}
	defer tty.Close()

	fd := tty.Fd()
	state, err := term.MakeRaw(fd)
	if err != nil {
		return false, false
	}
	defer term.Restore(fd, state)

	if _, err := tty.WriteString("\x1b]11;?\x07\x1b[c"); err != nil {
		return false, false
	}

	var buf []byte
	chunk := make([]byte, 256)
	deadline := time.Now().Add(themeQueryTimeout)
	for !da1Reply.Match(buf) {
		left := time.Until(deadline)
		if left <= 0 {
			break
		}
		var rfds unix.FdSet
		rfds.Set(int(fd))
		tv := unix.NsecToTimeval(left.Nanoseconds())
		n, err := unix.Select(int(fd)+1, &rfds, nil, nil, &tv)
		if err == unix.EINTR {
			continue
		}
		if err != nil || n == 0 {
			break
		}
		r, err := tty.Read(chunk)
		if err != nil {
			break
		}
		buf = append(buf, chunk[:r]...)
	}
	// Discard anything still queued. Bubble Tea swallows unknown CSI replies
	// but not OSC ones: a leftover OSC 11 answer would arrive as keystrokes
	// (alt+], the color text, ctrl+g). A reply arriving after this point can
	// still leak; the timeout above is what keeps that rare.
	for {
		var rfds unix.FdSet
		rfds.Set(int(fd))
		var zero unix.Timeval
		if n, err := unix.Select(int(fd)+1, &rfds, nil, nil, &zero); err != nil || n == 0 {
			break
		}
		r, err := tty.Read(chunk)
		if err != nil || r == 0 {
			break
		}
		buf = append(buf, chunk[:r]...)
	}
	return parseBackgroundReply(string(buf))
}
