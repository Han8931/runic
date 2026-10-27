package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/han/runic/internal/protocol"
)

// The New-session box's agent field: which coding agent the session should host.

func agentFormModel() model {
	m := model{groups: []string{"default", "work"}, agentKinds: []string{"claude", "codex", "gemini"}}
	m = m.openSessionForm(true, "", "")
	return m
}

// agentFieldIndex is where "agent" sits in the create form's focus order.
func agentFieldIndex(t *testing.T, m model) int {
	t.Helper()
	for i, f := range (&m.jobs.form).fields() {
		if f == "agent" {
			return i
		}
	}
	t.Fatal("the create form has no agent field")
	return -1
}

// Tab cycles the configured kinds, with "none" a real stop in the rotation —
// "actually, a plain session" is a normal correction to make.
func TestAgentFieldCyclesKinds(t *testing.T) {
	m := agentFormModel()
	m.jobs.form.setFocus(agentFieldIndex(t, m))

	press := func(m model, key tea.KeyMsg) model {
		got, _ := m.handleSessionFormKey(key)
		return got.(model)
	}
	tab := tea.KeyMsg{Type: tea.KeyTab}
	shiftTab := tea.KeyMsg{Type: tea.KeyShiftTab}

	m = press(m, tab)
	if got := m.jobs.form.agentInput.Value(); got != "claude" {
		t.Errorf("first tab = %q, want claude", got)
	}
	m = press(m, tab)
	m = press(m, tab)
	if got := m.jobs.form.agentInput.Value(); got != "gemini" {
		t.Errorf("third tab = %q, want gemini", got)
	}
	// Wrapping lands back on none rather than sticking at the end.
	m = press(m, tab)
	if got := m.jobs.form.agentInput.Value(); got != "" {
		t.Errorf("after the last kind, tab = %q, want none", got)
	}
	// Shift+tab backs through the list instead of leaving the field.
	m = press(m, shiftTab)
	if got := m.jobs.form.agentInput.Value(); got != "gemini" {
		t.Errorf("shift+tab = %q, want gemini", got)
	}
	if !(&m.jobs.form).agentFocused() {
		t.Error("cycling kinds must not move focus off the agent field")
	}
}

// ↑/↓ still move between fields while the agent field has focus — only tab is
// overloaded, the same compromise the group field makes.
func TestAgentFieldArrowsStillSwitchFields(t *testing.T) {
	m := agentFormModel()
	idx := agentFieldIndex(t, m)
	m.jobs.form.setFocus(idx)
	got, _ := m.handleSessionFormKey(tea.KeyMsg{Type: tea.KeyDown})
	down := got.(model)
	if (&down.jobs.form).agentFocused() {
		t.Error("down should leave the agent field")
	}
	got, _ = m.handleSessionFormKey(tea.KeyMsg{Type: tea.KeyUp})
	up := got.(model)
	if (&up.jobs.form).agentFocused() {
		t.Error("up should leave the agent field")
	}
}

// Choosing an agent prefills the branch with the session name: an agent usually
// wants its own worktree, and that is one keystroke instead of retyping a name.
func TestAgentChoicePrefillsBranch(t *testing.T) {
	m := agentFormModel()
	m.jobs.form.nameInput.SetValue("agent-1")
	m.jobs.form.setFocus(agentFieldIndex(t, m))

	got, _ := m.handleSessionFormKey(tea.KeyMsg{Type: tea.KeyTab})
	m = got.(model)
	if b := m.jobs.form.branchInput.Value(); b != "agent-1" {
		t.Errorf("branch = %q, want it prefilled with the session name", b)
	}
	// Setting the agent back to none clears it again, so declining an agent
	// never leaves a stray branch behind.
	for i := 0; i < len(m.agentKinds); i++ {
		got, _ = m.handleSessionFormKey(tea.KeyMsg{Type: tea.KeyTab})
		m = got.(model)
	}
	if a := m.jobs.form.agentInput.Value(); a != "" {
		t.Fatalf("expected to be back on none, got %q", a)
	}
	if b := m.jobs.form.branchInput.Value(); b != "" {
		t.Errorf("branch = %q, want it cleared with the agent", b)
	}
}

// A branch typed by hand is never overwritten: the prefill is a convenience,
// not a policy. An agent in the main checkout is a legitimate thing to want.
func TestTypedBranchSurvivesAgentChoice(t *testing.T) {
	m := agentFormModel()
	m.jobs.form.nameInput.SetValue("agent-1")

	// Type a branch by hand...
	branchIdx := agentFieldIndex(t, m) + 1
	m.jobs.form.setFocus(branchIdx)
	for _, r := range "fix/login" {
		got, _ := m.handleSessionFormKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = got.(model)
	}
	// ...then pick an agent.
	m.jobs.form.setFocus(agentFieldIndex(t, m))
	got, _ := m.handleSessionFormKey(tea.KeyMsg{Type: tea.KeyTab})
	m = got.(model)

	if b := m.jobs.form.branchInput.Value(); b != "fix/login" {
		t.Errorf("branch = %q, want the hand-typed name kept", b)
	}
	// And clearing the agent does not wipe it either.
	for i := 0; i < len(m.agentKinds); i++ {
		got, _ = m.handleSessionFormKey(tea.KeyMsg{Type: tea.KeyTab})
		m = got.(model)
	}
	if b := m.jobs.form.branchInput.Value(); b != "fix/login" {
		t.Errorf("branch = %q after clearing the agent, want it kept", b)
	}
}

// With the agent field focused, the box lists the kinds, so tab-cycling is a
// visible choice rather than a blind rotation.
func TestAgentFieldListsKinds(t *testing.T) {
	m := agentFormModel()
	m.jobs.form.setFocus(agentFieldIndex(t, m))
	out := stripAnsi(strings.Join(m.sessionFormLines(24, 60), "\n"))
	for _, want := range []string{"none", "claude", "codex", "gemini"} {
		if !strings.Contains(out, want) {
			t.Errorf("the agent strip should list %q: %q", want, out)
		}
	}

	// Hidden while another field has focus, like the group strip.
	m.jobs.form.setFocus(0)
	out = stripAnsi(strings.Join(m.sessionFormLines(24, 60), "\n"))
	if strings.Contains(out, "gemini") {
		t.Errorf("the agent strip should be hidden off the agent field: %q", out)
	}
}

// The edit form (`e` on an existing session) has no agent field: the kind is
// decided at creation, since it is what the first pane was started with.
func TestEditFormHasNoAgentField(t *testing.T) {
	m := model{agentKinds: []string{"claude"}}
	m = m.openSessionForm(false, "sess", "default")
	for _, f := range (&m.jobs.form).fields() {
		if f == "agent" {
			t.Error("the edit form should not offer an agent field")
		}
	}
}

// A typo in the free-text field is caught before the round trip, and says what
// would have worked.
func TestUnknownAgentKindRejectedWithKnownList(t *testing.T) {
	m := agentFormModel()
	m.jobs.form.nameInput.SetValue("a1")
	m.jobs.form.agentInput.SetValue("clade")
	got, cmd := m.submitSessionForm()
	m = got.(model)
	if cmd != nil {
		t.Error("a bad agent kind should not reach the daemon")
	}
	if !strings.Contains(m.status, "clade") || !strings.Contains(m.status, "claude") {
		t.Errorf("status = %q, want it to name the typo and the known kinds", m.status)
	}
	if m.jobs.form.active {
		// The form is cleared on submit; the message is how the user learns.
		t.Error("the form should not stay open")
	}
}

// The kind a session was created to host shows in its details — it is how you
// tell a claude session from a codex one — and is kept distinct from whatever
// its panes are currently reporting.
func TestSessionDetailNamesAgentKind(t *testing.T) {
	m := sideTabsModel()
	for i := range m.nodes {
		for j := range m.nodes[i].sessions {
			if m.nodes[i].sessions[j].Name == "build" {
				m.nodes[i].sessions[j].Agent = "claude"
				// Reporting something too, so the kind and the state are both
				// on the panel and have to be told apart.
				m.nodes[i].sessions[j].Attention = protocol.AttentionWaiting
				m.nodes[i].sessions[j].AttentionNote = "which schema?"
				m.nodes[i].sessions[j].AttentionSince = time.Now().Add(-time.Minute)
			}
		}
	}
	m.jobs.tree.tab = sideSpaces
	m.refreshTreeRows()
	for i, r := range m.jobs.tree.rows {
		if r.kind == treeSession && r.session == "build" {
			m.jobs.tree.cursor = i
		}
	}
	lines, _, ok := m.sessionDetailLines(60)
	if !ok {
		t.Fatal("expected a detail panel for the session row")
	}
	body := stripAnsi(strings.Join(lines, "\n"))
	if !strings.Contains(body, "claude") {
		t.Errorf("session detail should name the agent kind: %q", body)
	}
	// The attention line is labelled State, so it cannot be mistaken for the
	// kind sitting two lines above it.
	if !strings.Contains(body, "State") || !strings.Contains(body, "waiting") {
		t.Errorf("the reported state should be labelled State: %q", body)
	}
}

// With no registry configured the daemon is the authority, so the box must not
// refuse everything it cannot vouch for itself.
func TestEmptyRegistryDefersToDaemon(t *testing.T) {
	m := model{}
	m = m.openSessionForm(true, "", "")
	m.jobs.form.nameInput.SetValue("a1")
	m.jobs.form.agentInput.SetValue("something")
	_, cmd := m.submitSessionForm()
	if cmd == nil {
		t.Error("with no local registry the create should still be attempted")
	}
}
