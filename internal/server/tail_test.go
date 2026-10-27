package server

import (
	"strings"
	"testing"
)

// The tail has to read like a transcript, not like the byte stream it came
// from: colors, cursor moves and the OSC prompt markers runic itself injects
// are control traffic, and a progress bar's carriage returns would otherwise
// show every frame it ever drew.
func TestTailLinesReadsAsText(t *testing.T) {
	raw := []byte("\x1b[32mok\x1b[0m  github.com/x\n" +
		"\x1b]133;A\x07$ go test ./...\n" +
		"10%\r50%\r100% done\n" +
		"PASS\n\n\n")
	got := tailLines(raw, 10)
	want := []string{
		"ok  github.com/x",
		"$ go test ./...",
		"100% done",
		"PASS",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("tailLines =\n%q\nwant\n%q", got, want)
	}
}

func TestTailLinesKeepsOnlyTheTail(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 100; i++ {
		b.WriteString("line\n")
	}
	if got := tailLines([]byte(b.String()), 5); len(got) != 5 {
		t.Errorf("got %d lines, want 5", len(got))
	}
}

func TestTailLinesEmptyInputs(t *testing.T) {
	for _, raw := range []string{"", "\n\n\n", "\x1b[2J\x1b[H", "\x1b]0;title\x07"} {
		if got := tailLines([]byte(raw), 5); len(got) != 0 {
			t.Errorf("tailLines(%q) = %q, want nothing", raw, got)
		}
	}
}

func TestStripANSIKeepsTextDropsControl(t *testing.T) {
	cases := map[string]string{
		"\x1b[1;31mred\x1b[0m":        "red",
		"a\x1b[2Kb":                   "ab",
		"\x1b]133;A\x07prompt":        "prompt",
		"\x1b]8;;http://x\x1b\\link": "link",
		"plain":                       "plain",
		"tab\there":                   "tab\there",
	}
	for in, want := range cases {
		if got := stripANSI(in); got != want {
			t.Errorf("stripANSI(%q) = %q, want %q", in, got, want)
		}
	}
}

// A request is capped, so a client cannot ask the daemon to turn a megabyte of
// backlog into strings.
func TestTailRequestIsCapped(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	m := NewTerminalManager()
	term, err := m.GetOrCreate("s", "main", 80, 24)
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	defer m.KillAll()

	var b strings.Builder
	for i := 0; i < 500; i++ {
		b.WriteString("noise\n")
	}
	term.broadcast([]byte(b.String()))

	for _, ask := range []int{0, -1, 10_000} {
		lines, ok := m.Tail("s", "main", ask)
		if !ok {
			t.Fatal("expected the pane to answer")
		}
		if len(lines) > agentTailLines {
			t.Errorf("asked for %d, got %d lines (cap %d)", ask, len(lines), agentTailLines)
		}
	}
	if _, ok := m.Tail("s", "nosuch", 5); ok {
		t.Error("a pane that does not exist should not answer")
	}
}
