package tui

import "testing"

func TestParseBackgroundReply(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		dark, ok bool
	}{
		{"light 16-bit BEL", "\x1b]11;rgb:ffff/fafa/f0f0\x07\x1b[?62;22c", false, true},
		{"dark 16-bit ST", "\x1b]11;rgb:1e1e/1e1e/2e2e\x1b\\\x1b[?1;2c", true, true},
		{"8-bit components", "\x1b]11;rgb:ff/fa/f0\x07", false, true},
		{"4-bit components", "\x1b]11;rgb:0/0/0\x07", true, true},
		{"rgba with alpha", "\x1b]11;rgba:ffff/ffff/ffff/ffff\x07", false, true},
		{"uppercase hex", "\x1b]11;rgb:FFFF/FAFA/F0F0\x07", false, true},
		{"uppercase prefix", "\x1b]11;RGB:ffff/fafa/f0f0\x07", false, true},
		{"no terminator yet", "\x1b]11;rgb:1e1e/1e1e/2e2e", true, true},
		{"only DA1, no OSC answer", "\x1b[?1;2c", false, false},
		{"empty", "", false, false},
	}
	for _, c := range cases {
		dark, ok := parseBackgroundReply(c.in)
		if dark != c.dark || ok != c.ok {
			t.Errorf("%s: got dark=%v ok=%v, want dark=%v ok=%v", c.name, dark, ok, c.dark, c.ok)
		}
	}
}
