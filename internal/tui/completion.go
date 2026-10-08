package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Tab completion for the `:` command line, shared by both command modes: the
// pane view's (Ctrl+B c) and the management view's (`:`).
//
// Behavior follows the shell/vim convention the rest of the TUI already borrows
// from: the first tab completes as far as it unambiguously can, and once there
// is nothing left to add, further tabs cycle through the candidates (shift+tab
// cycles back). Candidates are listed under the input, mirroring the group strip
// in the session form.

// commandSpec describes one command for completion.
type commandSpec struct {
	// offer lists the spellings suggested as candidates, best first. A command
	// with several documented spellings (`:vs` and `:vsplit`) offers both.
	offer []string
	// alts lists other spellings the parser accepts. They are not suggested —
	// the strip would be mostly one-letter noise — but they still resolve to
	// this spec so `:h <tab>` completes h's arguments.
	alts []string
	// args are the static candidates for the command's first argument.
	args []string
	// path marks a command whose argument is a filesystem path.
	path bool
}

// matches reports whether spelling names this command.
func (c commandSpec) matches(spelling string) bool {
	for _, n := range c.offer {
		if n == spelling {
			return true
		}
	}
	for _, a := range c.alts {
		if a == spelling {
			return true
		}
	}
	return false
}

// jobsCommands is the completion table for the management view's `:` commands,
// kept in sync with executeJobsCommand. Alphabetical, so the candidate strip is
// in a predictable order.
var jobsCommands = []commandSpec{
	{offer: []string{"clear"}},
	{offer: []string{"config"}, args: []string{"logdir", "slots", "sort"}},
	{offer: []string{"help"}, alts: []string{"h", "?"}},
	{offer: []string{"kill"}, args: []string{"--all"}},
	{offer: []string{"logdir"}, path: true},
	{offer: []string{"parallel"}, alts: []string{"p"}},
	{offer: []string{"quit"}, alts: []string{"q"}},
	{offer: []string{"reset"}},
	{offer: []string{"restart"}, args: []string{"--all"}},
	{offer: []string{"set"}, args: []string{"logdir", "slots", "sort"}},
	{offer: []string{"slots"}},
	{offer: []string{"sort"}, args: []string{"group", "id", "state", "time"}},
}

// paneCommands is the completion table for the pane view's command line, kept in
// sync with executeCommand. The short forms are offered alongside the long ones
// because the short forms are what the docs use.
var paneCommands = []commandSpec{
	{offer: []string{"close"}, alts: []string{"x", "kill-pane"}},
	{offer: []string{"detach"}, alts: []string{"d"}},
	{offer: []string{"hs", "hsplit"}, alts: []string{"sp", "split"}},
	{offer: []string{"jobs"}, alts: []string{"j"}},
	{offer: []string{"next"}, alts: []string{"o", "next-pane"}},
	{offer: []string{"quit"}, alts: []string{"q"}},
	{offer: []string{"vs", "vsplit"}, alts: []string{"vsp"}},
}

// completionState tracks an in-progress completion so repeated tabs cycle
// through the candidates instead of re-completing the word a previous tab
// already filled in.
type completionState struct {
	// candidates is the last computed candidate list, shown under the input.
	candidates []string
	// listed is set when candidates are on screen but none has been inserted —
	// what the first tab on an empty word does. The next tab starts cycling.
	listed bool
	// cycling is set once the common prefix has been exhausted and tabs have
	// started replacing the word outright.
	cycling bool
	index   int
	// base is the input text before the word being completed; each cycle step
	// rebuilds the line from it rather than from the previous completion.
	base string
}

// reset clears completion state. Called on any key other than tab, so a line
// edited after a completion starts fresh instead of cycling from a stale base.
func (c *completionState) reset() { *c = completionState{} }

// completeCommand advances tab completion on the command line, updating the
// input in place. back cycles to the previous candidate.
func (m *model) completeCommand(table []commandSpec, back bool) {
	// Candidates are already on screen for this word: step through them.
	if len(m.completion.candidates) > 1 && (m.completion.cycling || m.completion.listed) {
		n := len(m.completion.candidates)
		if m.completion.listed {
			// Stepping out of a bare listing: land on the first candidate.
			m.completion.listed, m.completion.cycling = false, true
			m.completion.index = 0
			if back {
				m.completion.index = n - 1
			}
		} else {
			step := 1
			if back {
				step = -1
			}
			m.completion.index = ((m.completion.index+step)%n + n) % n
		}
		m.setCommandLine(m.completion.base + m.completion.candidates[m.completion.index])
		return
	}

	value := m.cmdInput.Value()
	base, partial := splitLastWord(value)

	// A path argument is completed against the filesystem; pathCandidates
	// already returns entries that start with what was typed.
	var cands []string
	if wantsPath(table, value) {
		cands = pathCandidates(partial)
	} else {
		cands = filterPrefix(commandCandidates(table, value), partial)
	}

	switch len(cands) {
	case 0:
		m.completion.reset()
		if partial != "" {
			m.status = "no completion for " + partial
		}
	case 1:
		// Unambiguous: fill it in and add a space so the next tab completes the
		// following word. A directory keeps its trailing separator instead, so
		// the next tab descends into it.
		m.completion.reset()
		if strings.HasSuffix(cands[0], string(filepath.Separator)) {
			m.setCommandLine(base + cands[0])
		} else {
			m.setCommandLine(base + cands[0] + " ")
		}
	default:
		m.completion.candidates = cands
		m.completion.base = base
		if lcp := longestCommonPrefix(cands); len(lcp) > len(partial) {
			// Still something unambiguous to add; show the candidates but don't
			// start cycling yet — the next tab will.
			m.completion.cycling, m.completion.listed = false, false
			m.setCommandLine(base + lcp)
			return
		}
		if partial == "" {
			// Nothing typed to narrow on, so list the options instead of
			// silently picking one: tab on an empty word must not pre-fill a
			// command the user never asked for (`:clear` is a single reflexive
			// Enter away from dropping every finished job). The next tab cycles.
			m.completion.listed = true
			return
		}
		m.completion.cycling = true
		m.completion.index = 0
		if back {
			m.completion.index = len(cands) - 1
		}
		m.setCommandLine(base + cands[m.completion.index])
	}
}

// setCommandLine replaces the command line's contents and parks the cursor at
// the end, where a completion leaves off.
func (m *model) setCommandLine(value string) {
	m.cmdInput.SetValue(value)
	m.cmdInput.CursorEnd()
}

// splitLastWord splits input into everything before the final word and the
// final word itself. Input ending in a space is completing a fresh word, so the
// partial is empty and the whole input is the base.
func splitLastWord(input string) (base, partial string) {
	idx := strings.LastIndexAny(input, " \t")
	if idx < 0 {
		return "", input
	}
	return input[:idx+1], input[idx+1:]
}

// commandCandidates returns the completion pool for the word at the end of
// input: command names at the start of the line, otherwise the arguments of the
// command already typed.
func commandCandidates(table []commandSpec, input string) []string {
	fields := strings.Fields(input)
	// The word being completed is the position after the complete words. A
	// trailing space means the final field is finished and a new word is next.
	pos := len(fields)
	if pos > 0 && !endsWithSpace(input) {
		pos--
	}
	if pos == 0 {
		return commandNames(table)
	}

	spec, ok := specFor(table, strings.ToLower(fields[0]))
	if !ok {
		return nil
	}
	// `:set`/`:config` take a setting name, then that setting's own arguments —
	// `:set sort <tab>` must offer the sort modes, not the setting names again.
	if len(spec.args) > 0 && pos >= 2 && len(fields) >= 2 {
		if sub, subOK := specFor(table, strings.ToLower(fields[1])); subOK && isSettingPrefix(fields[0]) {
			return argCandidates(sub, pos-1)
		}
	}
	return argCandidates(spec, pos)
}

// isSettingPrefix reports whether a command is one of the optional `set`/
// `config` prefixes, whose first argument is itself a command name.
func isSettingPrefix(cmd string) bool {
	c := strings.ToLower(cmd)
	return c == "set" || c == "config"
}

// argCandidates returns the candidates for a command's argument at position
// pos (1 = its first argument).
func argCandidates(spec commandSpec, pos int) []string {
	if spec.path {
		return nil // paths are completed separately, against the filesystem
	}
	if pos == 1 {
		return spec.args
	}
	return nil
}

// commandNames returns every suggested command spelling, sorted so the strip
// reads predictably.
func commandNames(table []commandSpec) []string {
	var out []string
	for _, c := range table {
		out = append(out, c.offer...)
	}
	sort.Strings(out)
	return out
}

func specFor(table []commandSpec, spelling string) (commandSpec, bool) {
	for _, c := range table {
		if c.matches(spelling) {
			return c, true
		}
	}
	return commandSpec{}, false
}

// wantsPath reports whether the word at the end of input is a filesystem path
// argument, so the caller completes it against the filesystem instead.
func wantsPath(table []commandSpec, input string) bool {
	fields := strings.Fields(input)
	pos := len(fields)
	if pos > 0 && !endsWithSpace(input) {
		pos--
	}
	if pos == 0 || len(fields) == 0 {
		return false
	}
	name := strings.ToLower(fields[0])
	if isSettingPrefix(name) && len(fields) >= 2 {
		spec, ok := specFor(table, strings.ToLower(fields[1]))
		return ok && spec.path && pos >= 2
	}
	spec, ok := specFor(table, name)
	return ok && spec.path && pos == 1
}

// pathCandidates lists the filesystem entries that could complete partial.
// Only directories are offered: every path argument on the command line (just
// the log directory today) names one.
func pathCandidates(partial string) []string {
	dir, prefix := filepath.Split(expandHome(partial))
	if dir == "" {
		dir = "."
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		// Rebuild against what the user actually typed, so a completion keeps
		// their "~/" or relative spelling instead of expanding it under them.
		typedDir, _ := filepath.Split(partial)
		out = append(out, typedDir+e.Name()+string(filepath.Separator))
	}
	sort.Strings(out)
	return out
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

func endsWithSpace(s string) bool {
	return strings.HasSuffix(s, " ") || strings.HasSuffix(s, "\t")
}

// filterPrefix keeps the candidates starting with partial, case-insensitively
// (commands are matched case-insensitively when executed too).
func filterPrefix(cands []string, partial string) []string {
	if partial == "" {
		return cands
	}
	lower := strings.ToLower(partial)
	var out []string
	for _, c := range cands {
		if strings.HasPrefix(strings.ToLower(c), lower) {
			out = append(out, c)
		}
	}
	return out
}

// longestCommonPrefix returns the longest prefix shared by every candidate —
// how far a single tab can fill in without guessing.
func longestCommonPrefix(cands []string) string {
	if len(cands) == 0 {
		return ""
	}
	lcp := cands[0]
	for _, c := range cands[1:] {
		i := 0
		for i < len(lcp) && i < len(c) && lcp[i] == c[i] {
			i++
		}
		lcp = lcp[:i]
	}
	return lcp
}

// completionHeight is how many lines the candidate strip occupies, so the
// layouts can give up exactly that much room instead of letting the strip push
// the footer off the bottom.
func (m model) completionHeight() int {
	if m.width <= 0 {
		return 0
	}
	return len(m.completionLines(m.width))
}

// completionLines renders the candidate strip shown under the command line: the
// candidates wrapped to the available width, with the current one highlighted
// while cycling. Empty when there is nothing to disambiguate.
func (m model) completionLines(width int) []string {
	cands := m.completion.candidates
	if len(cands) < 2 {
		return nil
	}
	current := ""
	if m.completion.cycling && m.completion.index < len(cands) {
		current = cands[m.completion.index]
	}
	var lines []string
	line, lineW := "  ", 2
	for _, c := range cands {
		tok, tokW := treeSummaryStyle.Render(c), lipgloss.Width(c)
		if c == current {
			tok, tokW = selectedStyle.Render(" "+c+" "), tokW+2
		}
		if lineW+tokW > width && lineW > 2 {
			lines = append(lines, fitToWidth(line, width))
			line, lineW = "  ", 2
		}
		line += tok + "  "
		lineW += tokW + 2
	}
	return append(lines, fitToWidth(line, width))
}
