package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/han/runic/internal/client"
	"github.com/han/runic/internal/protocol"
)

// The edit modals: the session/job edit box (`e`, `n`), the group rename
// variant, and the `:config` settings box.

// configForm is the settings modal opened by `:config`: the daemon options
// that are editable at runtime (parallel slots, log directory).
type configForm struct {
	active      bool
	slotsInput  textinput.Model
	logdirInput textinput.Model
	origSlots   int
	origLogdir  string
	focusField  int // 0 = slots, 1 = logdir
}

// sessionForm is the edit modal opened with `e`. On a session row (or when
// creating) it edits the session's name + group; on a job row it grows a job
// name field on top, so one box edits everything about the row.
type sessionForm struct {
	active       bool
	creating     bool // blank form → SessionCreate; else rename/move
	groupRename  bool // single-field variant: rename the group in origName
	jobID        int  // >= 0: job row — show the job-name field; -1 otherwise
	origLabel    string
	origName     string
	origGroup    string
	origTimeout  string
	labelInput   textinput.Model // job name; only rendered when jobID >= 0
	nameInput    textinput.Model // session name
	groupInput   textinput.Model
	timeoutInput textinput.Model // job timeout duration; empty = none
	// agentInput appears only when creating. A kind there ("claude") makes the
	// session host that coding agent, started in its first pane; blank makes a
	// plain session of shells. Tab cycles the configured kinds, the same way the
	// group field cycles existing groups.
	agentInput textinput.Model
	// branchInput appears only when creating. A branch name there makes the
	// session an isolated git worktree on that branch; blank makes an ordinary
	// session. The name is required rather than defaulted so the branch you will
	// later look for in git is one you chose.
	branchInput textinput.Model
	focusField  int // index into inputs()
	// branchTouched records that the branch field was edited by hand, so
	// choosing an agent stops prefilling it and never overwrites a typed name.
	branchTouched bool
}

// fields returns the form's field labels in focus order.
func (f *sessionForm) fields() []string {
	switch {
	case f.groupRename:
		return []string{"name"}
	case f.jobID >= 0:
		return []string{"name", "session", "group", "timeout"}
	case f.creating:
		return []string{"name", "group", "agent", "branch"}
	default:
		return []string{"name", "group"}
	}
}

// inputs returns pointers to the form's visible fields in focus order.
func (f *sessionForm) inputs() []*textinput.Model {
	switch {
	case f.groupRename:
		return []*textinput.Model{&f.nameInput}
	case f.jobID >= 0:
		return []*textinput.Model{&f.labelInput, &f.nameInput, &f.groupInput, &f.timeoutInput}
	case f.creating:
		return []*textinput.Model{&f.nameInput, &f.groupInput, &f.agentInput, &f.branchInput}
	default:
		return []*textinput.Model{&f.nameInput, &f.groupInput}
	}
}

// setFocus focuses field i (wrapping) and blurs the rest.
func (f *sessionForm) setFocus(i int) {
	inputs := f.inputs()
	n := len(inputs)
	f.focusField = ((i % n) + n) % n
	for idx, ti := range inputs {
		if idx == f.focusField {
			ti.Focus()
		} else {
			ti.Blur()
		}
	}
}

// focusedField names the field with focus, or "" when the form has none.
func (f *sessionForm) focusedField() string {
	fs := f.fields()
	if f.focusField < 0 || f.focusField >= len(fs) {
		return ""
	}
	return fs[f.focusField]
}

// groupFocused reports whether the group field has focus.
func (f *sessionForm) groupFocused() bool { return f.focusedField() == "group" }

// agentFocused reports whether the agent field has focus.
func (f *sessionForm) agentFocused() bool { return f.focusedField() == "agent" }

func formInput(value string) textinput.Model {
	ti := newTextInput()
	ti.CharLimit = 64
	ti.Prompt = ""
	ti.SetValue(value)
	ti.CursorEnd()
	return ti
}

// openSessionForm shows the create/edit session modal, pre-filled when editing.
func (m model) openSessionForm(creating bool, name, group string) model {
	ni := formInput(name)
	ni.Focus()
	m.jobs.form = sessionForm{
		active:      true,
		creating:    creating,
		jobID:       -1,
		origName:    name,
		origGroup:   group,
		nameInput:   ni,
		groupInput:  formInput(group),
		agentInput:  formInput(""),
		branchInput: formInput(""),
	}
	return m
}

// openGroupRenameForm shows the one-field modal that renames a group.
func (m model) openGroupRenameForm(name string) model {
	ni := formInput(name)
	ni.Focus()
	m.jobs.form = sessionForm{
		active:      true,
		groupRename: true,
		jobID:       -1,
		origName:    name,
		nameInput:   ni,
	}
	return m
}

// openJobEditForm shows the combined edit modal for a job row: the job's name
// plus its session's name and group.
func (m model) openJobEditForm(j protocol.JobInfo, group string) model {
	li := formInput(j.Label)
	li.Focus()
	timeout := ""
	if j.TimeoutMS > 0 {
		timeout = durationCompact(time.Duration(j.TimeoutMS) * time.Millisecond)
	}
	m.jobs.form = sessionForm{
		active:       true,
		jobID:        j.ID,
		origLabel:    j.Label,
		origName:     j.Session,
		origGroup:    group,
		origTimeout:  timeout,
		labelInput:   li,
		nameInput:    formInput(j.Session),
		groupInput:   formInput(group),
		timeoutInput: formInput(timeout),
	}
	return m
}

func (m model) handleSessionFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := &m.jobs.form
	switch msg.String() {
	case "esc":
		m.jobs.form = sessionForm{}
		return m, nil
	case "tab":
		// On the group and agent fields, tab cycles through the known values so
		// they are discoverable; ↑/↓ and shift+tab still switch fields.
		if f.groupFocused() {
			if g, ok := nextGroup(m.groups, strings.TrimSpace(f.groupInput.Value()), 1); ok {
				f.groupInput.SetValue(g)
				f.groupInput.CursorEnd()
			}
			return m, nil
		}
		if f.agentFocused() {
			m.cycleFormAgent(1)
			return m, nil
		}
		f.setFocus(f.focusField + 1)
		return m, textinput.Blink
	case "down":
		f.setFocus(f.focusField + 1)
		return m, textinput.Blink
	case "shift+tab":
		// Shift+tab backs through the agent kinds while that field is focused,
		// so a four-item list is reachable in one step from either end.
		if f.agentFocused() {
			m.cycleFormAgent(-1)
			return m, nil
		}
		f.setFocus(f.focusField - 1)
		return m, textinput.Blink
	case "up":
		f.setFocus(f.focusField - 1)
		return m, textinput.Blink
	case "enter":
		return m.submitSessionForm()
	}
	var cmd tea.Cmd
	in := f.inputs()[f.focusField]
	field := f.focusedField()
	*in, cmd = in.Update(msg)
	// A hand-edited branch is never prefilled over: picking an agent after
	// typing a branch must not replace what you typed.
	if field == "branch" {
		f.branchTouched = true
	}
	// The branch tracks the session name too, since the name is usually typed
	// after the agent is picked.
	if field == "agent" || field == "name" {
		m.syncFormBranchToAgent()
	}
	return m, cmd
}

// cycleFormAgent steps the agent field through "none" and the configured kinds,
// wrapping. Blank is a real entry in that rotation rather than something you
// have to clear by hand, because "actually, a plain session" is a normal
// correction to make.
func (m *model) cycleFormAgent(dir int) {
	f := &m.jobs.form
	choices := append([]string{""}, m.agentKinds...)
	cur := strings.TrimSpace(f.agentInput.Value())
	at := 0
	for i, c := range choices {
		if strings.EqualFold(c, cur) {
			at = i
			break
		}
	}
	n := len(choices)
	f.agentInput.SetValue(choices[((at+dir)%n+n)%n])
	f.agentInput.CursorEnd()
	m.syncFormBranchToAgent()
}

// syncFormBranchToAgent prefills the branch with the session name once an agent
// is chosen, since an agent usually wants its own worktree — but only while the
// branch is still untouched, and it clears again if the agent is set back to
// none, so the box never silently commits you to a git branch you did not ask
// for.
func (m *model) syncFormBranchToAgent() {
	f := &m.jobs.form
	if !f.creating || f.branchTouched {
		return
	}
	if strings.TrimSpace(f.agentInput.Value()) == "" {
		f.branchInput.SetValue("")
		return
	}
	f.branchInput.SetValue(strings.TrimSpace(f.nameInput.Value()))
	f.branchInput.CursorEnd()
}

// knownAgentKind reports whether kind is in the configured registry, matched
// the way the daemon matches it. It is a courtesy check that turns a typo in
// the free-text field into an immediate message instead of a round trip; the
// daemon stays the authority, so an empty local registry accepts anything
// rather than refusing every agent.
func (m model) knownAgentKind(kind string) bool {
	if len(m.agentKinds) == 0 {
		return true
	}
	for _, k := range m.agentKinds {
		if strings.EqualFold(k, kind) {
			return true
		}
	}
	return false
}

// nextGroup returns the entry dir (±1) steps after cur in groups, wrapping.
// A value not in the list (or blank) starts from the first/last group.
func nextGroup(groups []string, cur string, dir int) (string, bool) {
	if len(groups) == 0 {
		return "", false
	}
	idx := -1
	for i, g := range groups {
		if g == cur {
			idx = i
			break
		}
	}
	if idx < 0 {
		if dir < 0 {
			return groups[len(groups)-1], true
		}
		return groups[0], true
	}
	return groups[(idx+dir+len(groups))%len(groups)], true
}

func (m model) submitSessionForm() (tea.Model, tea.Cmd) {
	f := m.jobs.form
	name := strings.TrimSpace(f.nameInput.Value())
	group := strings.TrimSpace(f.groupInput.Value())
	m.jobs.form = sessionForm{}
	if name == "" {
		m.status = "session name required"
		return m, nil
	}
	if f.groupRename {
		if name == f.origName {
			return m, nil
		}
		return m, renameGroupCmd(f.origName, name)
	}
	if f.creating {
		agent := strings.TrimSpace(f.agentInput.Value())
		if agent != "" && !m.knownAgentKind(agent) {
			m.status = fmt.Sprintf("unknown agent %q (known: %s)",
				agent, strings.Join(m.agentKinds, ", "))
			return m, nil
		}
		// Open the new session as soon as the next tree refresh includes it.
		m.pendingOpen = name
		branch := strings.TrimSpace(f.branchInput.Value())
		return m, createSessionInGroup(name, group, branch, agent)
	}

	var cmds []tea.Cmd
	if f.jobID >= 0 {
		if label := strings.TrimSpace(f.labelInput.Value()); label != f.origLabel {
			cmds = append(cmds, setJobLabelCmd(f.jobID, label))
		}
		if timeout := strings.TrimSpace(f.timeoutInput.Value()); timeout != f.origTimeout {
			var ms int64
			if timeout != "" && !strings.EqualFold(timeout, "none") {
				d, err := time.ParseDuration(timeout)
				if err != nil || d <= 0 {
					m.status = fmt.Sprintf("invalid timeout %q (e.g. 30m, 90s)", timeout)
					return m, nil
				}
				ms = d.Milliseconds()
			}
			cmds = append(cmds, setJobTimeoutCmd(f.jobID, ms))
		}
	}
	if name != f.origName || group != f.origGroup {
		cmds = append(cmds, editSession(f.origName, name, f.origGroup, group))
	}
	if len(cmds) == 0 {
		return m, nil
	}
	return m, tea.Batch(cmds...)
}

// --- settings form (`:config`) --------------------------------------------

// settingsMsg carries the fetched daemon settings that seed the `:config`
// modal (slots come from the model's own tree poll).
type settingsMsg struct {
	logdir string
}

// loadSettingsCmd fetches the daemon-side settings the modal edits.
func loadSettingsCmd() tea.Cmd {
	return func() tea.Msg {
		dir, err := client.GetLogdir()
		if err != nil {
			return actionDoneMsg{err: err}
		}
		return settingsMsg{logdir: dir}
	}
}

// openSettingsForm shows the `:config` modal pre-filled with current values.
func (m model) openSettingsForm(logdir string) model {
	si := newTextInput()
	si.CharLimit = 4
	si.Prompt = ""
	si.SetValue(strconv.Itoa(m.maxSlots))
	si.CursorEnd()
	si.Focus()
	li := newTextInput()
	li.CharLimit = 256
	li.Prompt = ""
	li.SetValue(logdir)
	li.CursorEnd()
	m.jobs.settings = configForm{
		active:      true,
		slotsInput:  si,
		logdirInput: li,
		origSlots:   m.maxSlots,
		origLogdir:  logdir,
	}
	return m
}

func (f *configForm) toggleField() {
	f.focusField = 1 - f.focusField
	if f.focusField == 0 {
		f.slotsInput.Focus()
		f.logdirInput.Blur()
	} else {
		f.logdirInput.Focus()
		f.slotsInput.Blur()
	}
}

func (m model) handleSettingsFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := &m.jobs.settings
	switch msg.String() {
	case "esc":
		m.jobs.settings = configForm{}
		return m, nil
	case "tab", "shift+tab", "up", "down":
		f.toggleField()
		return m, textinput.Blink
	case "enter":
		return m.submitSettingsForm()
	}
	var cmd tea.Cmd
	if f.focusField == 0 {
		f.slotsInput, cmd = f.slotsInput.Update(msg)
	} else {
		f.logdirInput, cmd = f.logdirInput.Update(msg)
	}
	return m, cmd
}

func (m model) submitSettingsForm() (tea.Model, tea.Cmd) {
	f := m.jobs.settings
	m.jobs.settings = configForm{}

	n, err := strconv.Atoi(strings.TrimSpace(f.slotsInput.Value()))
	if err != nil || n < 1 {
		m.status = fmt.Sprintf("invalid slot count: %q", f.slotsInput.Value())
		return m, nil
	}
	logdir := strings.TrimSpace(f.logdirInput.Value())

	var cmds []tea.Cmd
	if n != f.origSlots {
		cmds = append(cmds, setMaxSlotsCmd(n))
	}
	if logdir != "" && logdir != f.origLogdir {
		cmds = append(cmds, setLogdirCmd(logdir))
	}
	if len(cmds) == 0 {
		return m, nil
	}
	return m, tea.Batch(cmds...)
}

// sessionFormLines renders the edit modal centered in the body: session name +
// group, preceded by the job-name field when a job row is being edited.
func (m model) sessionFormLines(bodyH, inner int) []string {
	f := m.jobs.form
	title := "Edit session"
	switch {
	case f.creating:
		title = "New session"
	case f.groupRename:
		title = "Rename group"
	case f.jobID >= 0:
		title = fmt.Sprintf("Edit job %d", f.jobID)
	}
	boxInner := modalInnerWidth(inner, 46)
	const labelW = 7
	valueW := boxInner - 2 - labelW - 2 // marker(2) + label + gap(2)
	if valueW < 8 {
		valueW = 8
	}
	field := func(label string, ti textinput.Model, focused bool) string {
		ti.Width = valueW - 1 // textinput renders Width+1 cells (the cursor block)
		marker := "  "
		lbl := fmt.Sprintf("%-*s", labelW, label)
		if focused {
			marker = modalTitleStyle.Render("▌ ")
			lbl = modalActiveStyle.Render(lbl)
		} else {
			lbl = jobsDetailKeyStyle.Render(lbl)
		}
		return marker + lbl + "  " + ti.View()
	}

	labels := (&f).fields()
	inputs := (&f).inputs()
	content := []string{""}
	for i, lbl := range labels {
		content = append(content, field(lbl, *inputs[i], f.focusField == i))
	}
	// While the group or agent field is focused, show its choices as a strip so
	// tab-cycling is a visible choice, not a blind rotation.
	switch {
	case f.groupFocused():
		choices := groupChoiceLines(m.groups, strings.TrimSpace(f.groupInput.Value()), boxInner-2)
		if len(choices) > 0 {
			content = append(content, "")
			content = append(content, choices...)
		}
	case f.agentFocused():
		// "none" leads the list because a plain session is still the common
		// case, and it has to be visibly one of the options.
		kinds := append([]string{"none"}, m.agentKinds...)
		cur := strings.TrimSpace(f.agentInput.Value())
		if cur == "" {
			cur = "none"
		}
		choices := groupChoiceLines(kinds, cur, boxInner-2)
		if len(choices) > 0 {
			content = append(content, "")
			content = append(content, choices...)
		}
	}
	// Neither the agent nor the branch field explains itself from its label, so
	// say what they do where they are in use.
	if f.creating {
		hint := "  branch: blank for a plain session, or a name to isolate it in its own git worktree"
		if f.agentFocused() {
			hint = "  agent: started in the session's first pane, which keeps a shell underneath it"
		}
		content = append(content, "")
		content = append(content, wrapHelpLines(hint, boxInner-2)...)
	}
	keys := "  tab: groups · ↑/↓: switch · ⏎: save · esc: cancel"
	if f.agentFocused() {
		keys = "  tab/shift+tab: agent · ↑/↓: switch · ⏎: save · esc: cancel"
	}
	content = append(content, "", helpStyle.Render(keys))
	return centerBox(modalBox(title, content, boxInner), bodyH, inner)
}

// wrapHelpLines breaks a help sentence onto as many dim lines as it needs,
// keeping the leading indent on each so the block reads as one note.
func wrapHelpLines(text string, width int) []string {
	if width < 8 {
		return nil
	}
	indent := text[:len(text)-len(strings.TrimLeft(text, " "))]
	var lines []string
	line := indent
	for _, word := range strings.Fields(text) {
		candidate := line + word
		if len(line) > len(indent) {
			candidate = line + " " + word
		}
		if lipgloss.Width(candidate) > width && len(line) > len(indent) {
			lines = append(lines, helpStyle.Render(fitToWidth(line, width)))
			line = indent + word
			continue
		}
		line = candidate
	}
	if strings.TrimSpace(line) != "" {
		lines = append(lines, helpStyle.Render(fitToWidth(line, width)))
	}
	return lines
}

// groupChoiceLines renders the existing groups as wrapped strip lines; the
// entry matching the group field's current value is highlighted.
func groupChoiceLines(groups []string, current string, width int) []string {
	if len(groups) == 0 {
		return nil
	}
	var lines []string
	line, lineW := "  ", 2
	for _, g := range groups {
		tok, tokW := treeSummaryStyle.Render(g), lipgloss.Width(g)
		if g == current {
			tok, tokW = selectedStyle.Render(" "+g+" "), tokW+2
		}
		if lineW+tokW > width && lineW > 2 {
			lines = append(lines, line)
			line, lineW = "  ", 2
		}
		line += tok + "  "
		lineW += tokW + 2
	}
	return append(lines, line)
}

// settingsFormLines renders the `:config` settings modal centered in the body.
func (m model) settingsFormLines(bodyH, inner int) []string {
	f := m.jobs.settings
	boxInner := modalInnerWidth(inner, 46)
	const labelW = 6
	valueW := boxInner - 2 - labelW - 2
	if valueW < 8 {
		valueW = 8
	}
	field := func(label string, ti textinput.Model, focused bool) string {
		ti.Width = valueW - 1 // textinput renders Width+1 cells (the cursor block)
		marker := "  "
		lbl := fmt.Sprintf("%-*s", labelW, label)
		if focused {
			marker = modalTitleStyle.Render("▌ ")
			lbl = modalActiveStyle.Render(lbl)
		} else {
			lbl = jobsDetailKeyStyle.Render(lbl)
		}
		return marker + lbl + "  " + ti.View()
	}
	content := []string{
		"",
		field("slots", f.slotsInput, f.focusField == 0),
		field("logdir", f.logdirInput, f.focusField == 1),
		"",
		helpStyle.Render("  tab: switch · ⏎: save · esc: cancel"),
	}
	return centerBox(modalBox("Settings", content, boxInner), bodyH, inner)
}
