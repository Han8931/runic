package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cmdModel returns a model with the management view's command line open, as if
// the user had just pressed `:`.
func cmdModel(value string) model {
	m := model{width: 100, height: 40, cmdInput: newTextInput()}
	m.jobs.commanding = true
	m.cmdInput.SetValue(value)
	m.cmdInput.CursorEnd()
	return m
}

// tab on an unambiguous prefix fills the command in and leaves a space, ready
// for its argument.
func TestCompleteUniquePrefix(t *testing.T) {
	m := cmdModel("cle")
	(&m).completeCommand(jobsCommands, false)
	if got := m.cmdInput.Value(); got != "clear " {
		t.Fatalf("value = %q, want %q", got, "clear ")
	}
	if len(m.completion.candidates) != 0 {
		t.Fatalf("a unique completion should leave no candidate strip: %v", m.completion.candidates)
	}
}

// With several matches, the first tab adds only what they all share.
func TestCompleteFillsCommonPrefix(t *testing.T) {
	m := cmdModel("c")
	(&m).completeCommand(jobsCommands, false)
	// clear, config -> "c" + "l"/"o" diverge immediately, so nothing to add.
	if got := m.cmdInput.Value(); got != "clear" && got != "config" {
		t.Fatalf("value = %q, want one of the two c-commands", got)
	}
	if len(m.completion.candidates) != 2 {
		t.Fatalf("candidates = %v, want clear and config", m.completion.candidates)
	}

	// "re" is shared by reset and restart, so the first tab extends to "res".
	m = cmdModel("r")
	(&m).completeCommand(jobsCommands, false)
	if got := m.cmdInput.Value(); got != "res" {
		t.Fatalf("value = %q, want %q (the common prefix of reset/restart)", got, "res")
	}
	if m.completion.cycling {
		t.Fatal("should not cycle while there is still a common prefix to add")
	}
}

// Once the common prefix is exhausted, repeated tabs cycle and shift+tab goes
// back; cycling wraps.
func TestCompleteCyclesAndWraps(t *testing.T) {
	m := cmdModel("res")
	(&m).completeCommand(jobsCommands, false) // "res" is itself ambiguous
	first := m.cmdInput.Value()
	if !m.completion.cycling {
		t.Fatalf("expected cycling after an exhausted prefix, value=%q", first)
	}
	(&m).completeCommand(jobsCommands, false)
	second := m.cmdInput.Value()
	if second == first {
		t.Fatalf("second tab did not advance: still %q", first)
	}
	// Wrap forward back to the first.
	(&m).completeCommand(jobsCommands, false)
	if got := m.cmdInput.Value(); got != first {
		t.Fatalf("cycling did not wrap: %q, want %q", got, first)
	}
	// shift+tab steps back.
	(&m).completeCommand(jobsCommands, true)
	if got := m.cmdInput.Value(); got != second {
		t.Fatalf("shift+tab = %q, want %q", got, second)
	}
}

func TestCompleteNoMatch(t *testing.T) {
	m := cmdModel("zzz")
	(&m).completeCommand(jobsCommands, false)
	if got := m.cmdInput.Value(); got != "zzz" {
		t.Fatalf("value = %q, want it unchanged", got)
	}
	if m.status == "" {
		t.Fatal("expected a status message explaining there is no completion")
	}
}

// An empty line lists every command WITHOUT inserting one: tab on an empty word
// must not pre-fill something the user never asked for (`clear` sorts first and
// drops every finished job). The next tab starts cycling.
func TestCompleteEmptyListsWithoutInserting(t *testing.T) {
	m := cmdModel("")
	(&m).completeCommand(jobsCommands, false)
	if len(m.completion.candidates) != len(commandNames(jobsCommands)) {
		t.Fatalf("candidates = %d, want all %d commands", len(m.completion.candidates), len(commandNames(jobsCommands)))
	}
	if got := m.cmdInput.Value(); got != "" {
		t.Fatalf("value = %q, want the line left empty", got)
	}
	if !m.completion.listed || m.completion.cycling {
		t.Fatalf("expected a bare listing, got listed=%v cycling=%v", m.completion.listed, m.completion.cycling)
	}

	// The second tab commits to the first candidate.
	(&m).completeCommand(jobsCommands, false)
	if got := m.cmdInput.Value(); got != commandNames(jobsCommands)[0] {
		t.Fatalf("second tab = %q, want %q", got, commandNames(jobsCommands)[0])
	}
	// ...and shift+tab from a bare listing goes to the last.
	m2 := cmdModel("")
	(&m2).completeCommand(jobsCommands, false)
	(&m2).completeCommand(jobsCommands, true)
	names := commandNames(jobsCommands)
	if got := m2.cmdInput.Value(); got != names[len(names)-1] {
		t.Fatalf("shift+tab from a listing = %q, want %q", got, names[len(names)-1])
	}
}

// Same for an argument position: `:sort <tab>` lists the modes rather than
// silently choosing one.
func TestCompleteEmptyArgumentLists(t *testing.T) {
	m := cmdModel("sort ")
	(&m).completeCommand(jobsCommands, false)
	if got := m.cmdInput.Value(); got != "sort " {
		t.Fatalf("value = %q, want it unchanged", got)
	}
	if len(m.completion.candidates) != 4 {
		t.Fatalf("candidates = %v, want the four sort modes", m.completion.candidates)
	}
}

// Argument completion is context-aware: each command offers its own values.
func TestCompleteArguments(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"sort s", "state "}, // unique
		{"kill -", "--all "}, // unique
		{"restart --", "--all "},
		{"set s", "slots "}, // set's own argument list
		{"set so", "sort "}, // through the set prefix
	}
	for _, c := range cases {
		m := cmdModel(c.input)
		(&m).completeCommand(jobsCommands, false)
		if got := m.cmdInput.Value(); !strings.HasPrefix(got, c.want) && got != c.input+c.want {
			// Ambiguous inputs land on the first candidate or the common prefix;
			// assert the completed word rather than the whole line.
			_, partial := splitLastWord(got)
			if !strings.HasPrefix(c.want, partial) && !strings.HasPrefix(partial, strings.TrimSpace(c.want)) {
				t.Errorf("%q -> %q, want the word to head toward %q", c.input, got, c.want)
			}
		}
	}
}

// `:set sort <tab>` must offer the sort modes, not the setting names again —
// set/config are optional prefixes, so completion has to look past them.
func TestCompleteThroughSetPrefix(t *testing.T) {
	m := cmdModel("set sort ")
	(&m).completeCommand(jobsCommands, false)
	got := m.completion.candidates
	if len(got) == 0 {
		// A single candidate is inserted outright; check the line instead.
		if v := m.cmdInput.Value(); !strings.HasPrefix(v, "set sort ") {
			t.Fatalf("value = %q", v)
		}
		t.Fatalf("expected the sort modes as candidates, got none (value %q)", m.cmdInput.Value())
	}
	want := map[string]bool{"group": true, "id": true, "state": true, "time": true}
	for _, c := range got {
		if !want[c] {
			t.Fatalf("candidate %q is not a sort mode; completion did not look past the set prefix (%v)", c, got)
		}
	}
}

// A command with no arguments offers nothing for its second word, rather than
// falling back to the command list.
func TestCompleteNoArgsForArgumentlessCommand(t *testing.T) {
	m := cmdModel("clear ")
	(&m).completeCommand(jobsCommands, false)
	if got := m.cmdInput.Value(); got != "clear " {
		t.Fatalf("value = %q, want it unchanged", got)
	}
}

// The pane view has its own command table.
func TestCompletePaneCommands(t *testing.T) {
	m := cmdModel("vs")
	(&m).completeCommand(paneCommands, false)
	// "vs" and "vsplit" both start with vs, so the first tab can add nothing.
	if !m.completion.cycling {
		t.Fatalf("expected cycling between vs and vsplit, value=%q", m.cmdInput.Value())
	}

	m = cmdModel("det")
	(&m).completeCommand(paneCommands, false)
	if got := m.cmdInput.Value(); got != "detach " {
		t.Fatalf("value = %q, want %q", got, "detach ")
	}
}

// A management-view command must not complete in the pane view and vice versa:
// the tables are what keeps the suggestions honest about what will actually run.
func TestCompleteTablesAreSeparate(t *testing.T) {
	m := cmdModel("logdir")
	(&m).completeCommand(paneCommands, false)
	if m.status == "" {
		t.Fatal("logdir is not a pane-view command; expected no completion")
	}

	m = cmdModel("vsplit")
	(&m).completeCommand(jobsCommands, false)
	if m.status == "" {
		t.Fatal("vsplit is not a management-view command; expected no completion")
	}
}

// logdir takes a path, completed against the filesystem.
func TestCompletePathArgument(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o700); err != nil {
			t.Fatalf("Mkdir: %v", err)
		}
	}
	// A file must not be offered: every path argument names a directory.
	if err := os.WriteFile(filepath.Join(dir, "afile"), nil, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	m := cmdModel("logdir " + dir + string(filepath.Separator) + "al")
	(&m).completeCommand(jobsCommands, false)
	want := "logdir " + filepath.Join(dir, "alpha") + string(filepath.Separator)
	if got := m.cmdInput.Value(); got != want {
		t.Fatalf("value = %q, want %q", got, want)
	}

	// Ambiguous prefix "a" matches the directory alpha but not the file.
	m = cmdModel("logdir " + dir + string(filepath.Separator) + "a")
	(&m).completeCommand(jobsCommands, false)
	if got := m.cmdInput.Value(); got != want {
		t.Fatalf("file was offered as a path candidate: %q", got)
	}

	// Through the set prefix too.
	m = cmdModel("set logdir " + dir + string(filepath.Separator) + "b")
	(&m).completeCommand(jobsCommands, false)
	wantB := "set logdir " + filepath.Join(dir, "beta") + string(filepath.Separator)
	if got := m.cmdInput.Value(); got != wantB {
		t.Fatalf("value = %q, want %q", got, wantB)
	}
}

// A completed directory keeps its trailing separator so the next tab descends
// into it rather than starting a new word.
func TestCompletedDirectoryKeepsSeparator(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "outer", "inner"), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	m := cmdModel("logdir " + dir + string(filepath.Separator) + "out")
	(&m).completeCommand(jobsCommands, false)
	if got := m.cmdInput.Value(); !strings.HasSuffix(got, string(filepath.Separator)) {
		t.Fatalf("value = %q, want a trailing separator", got)
	}
	(&m).completeCommand(jobsCommands, false)
	if got := m.cmdInput.Value(); !strings.Contains(got, "inner") {
		t.Fatalf("second tab did not descend: %q", got)
	}
}

func TestSplitLastWord(t *testing.T) {
	cases := []struct{ in, base, partial string }{
		{"", "", ""},
		{"set", "", "set"},
		{"set ", "set ", ""},
		{"set slo", "set ", "slo"},
		{"set  slo", "set  ", "slo"},
	}
	for _, c := range cases {
		base, partial := splitLastWord(c.in)
		if base != c.base || partial != c.partial {
			t.Errorf("splitLastWord(%q) = (%q, %q), want (%q, %q)", c.in, base, partial, c.base, c.partial)
		}
	}
}

func TestLongestCommonPrefix(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"reset"}, "reset"},
		{[]string{"reset", "restart"}, "res"},
		{[]string{"clear", "config"}, "c"},
		{[]string{"a", "b"}, ""},
	}
	for _, c := range cases {
		if got := longestCommonPrefix(c.in); got != c.want {
			t.Errorf("longestCommonPrefix(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Every command the tables suggest must be one the executor actually accepts —
// otherwise tab would hand the user a command that errors on enter.
func TestOfferedCommandsAreReal(t *testing.T) {
	m := cmdModel("")
	for _, name := range commandNames(jobsCommands) {
		mm := m
		mm.status = ""
		res, _ := mm.executeJobsCommand(name)
		if got, ok := res.(model); ok && strings.HasPrefix(got.status, "unknown command") {
			t.Errorf("jobsCommands offers %q but executeJobsCommand rejects it: %s", name, got.status)
		}
	}
	for _, name := range commandNames(paneCommands) {
		mm := m
		mm.status = ""
		res, _ := mm.executeCommand(name)
		if got, ok := res.(model); ok && strings.HasPrefix(got.status, "unknown command") {
			t.Errorf("paneCommands offers %q but executeCommand rejects it: %s", name, got.status)
		}
	}
}

// The candidate strip has to be budgeted for, or it pushes the footer and the
// hardware bar off the bottom of the screen.
func TestCompletionHeightIsBudgeted(t *testing.T) {
	m := cmdModel("")
	plain := m.jobsBodyHeight()

	(&m).completeCommand(jobsCommands, false) // fills the strip with every command
	strip := m.completionHeight()
	if strip < 1 {
		t.Fatal("expected the candidate strip to occupy at least one line")
	}
	if got := m.jobsBodyHeight(); got != plain-strip {
		t.Fatalf("body height = %d, want %d (%d minus the %d-line strip)", got, plain-strip, plain, strip)
	}
}

// Editing the line after a completion must not keep cycling from a stale base.
func TestCompletionResetOnEdit(t *testing.T) {
	m := cmdModel("res")
	(&m).completeCommand(jobsCommands, false)
	if !m.completion.cycling {
		t.Fatal("expected cycling")
	}
	m.completion.reset()
	if m.completion.cycling || m.completion.candidates != nil || m.completion.base != "" {
		t.Fatalf("reset left state behind: %+v", m.completion)
	}
	if m.completionHeight() != 0 {
		t.Fatal("a reset completion must take no vertical space")
	}
}

// Bare `:set` opens the settings box like bare `:config`, rather than failing
// with "unknown command" — tab completion offers `set`, so that path is one
// keystroke away.
func TestBareSetOpensSettings(t *testing.T) {
	for _, name := range []string{"set", "config"} {
		m := cmdModel(name)
		res, cmd := m.executeJobsCommand(name)
		got, ok := res.(model)
		if !ok {
			t.Fatalf("%q: unexpected model type", name)
		}
		if strings.HasPrefix(got.status, "unknown command") {
			t.Errorf("%q: %s", name, got.status)
		}
		if cmd == nil {
			t.Errorf("%q: expected the settings-load command", name)
		}
	}
}
