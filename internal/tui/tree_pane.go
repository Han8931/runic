package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/han/runic/internal/protocol"
)

// treePane is the NerdTree-style sidebar in the MANAGE view: a foldable
// group → session → job tree for managing sessions and groups. The flat job
// list to its right is unchanged; this pane is purely for browsing/managing the
// hierarchy.
type treePane struct {
	show      bool
	focus     bool // keys drive the tree instead of the list
	zoom      bool // NerdTree-style `A`: expand the pane to fill the whole box
	cursor    int
	rows      []treeRow       // derived visible rows (honors fold state)
	collapsed map[string]bool // node key → folded; survives the live refresh
	menu      treeMenu        // NerdTree-style `m` action menu (when active)
}

// treeMenu is the NerdTree-style pop-up of context actions for the node under
// the cursor, opened with `m`. It captures its target row so the actions stay
// bound to it even though the cursor can't move while the menu is open.
type treeMenu struct {
	active bool
	row    treeRow
	items  []treeMenuItem
	cursor int
}

type treeMenuAct int

const (
	menuOpen   treeMenuAct = iota // fold group / open session / page job
	menuAdd                       // add a session (to this group)
	menuRename                    // rename the group or session
	menuDelete                    // delete the group or session
)

type treeMenuItem struct {
	key   string // single-key shortcut, shown as "(k)"
	label string
	act   treeMenuAct
}

type treeRowKind int

const (
	treeGroup treeRowKind = iota
	treeSession
	treeJob
)

type treeRow struct {
	kind    treeRowKind
	group   string
	session string
	job     protocol.JobInfo
	key     string // fold key (group/session rows); "" for jobs
	hasKids bool
	// attention is the session row's rolled-up pane state (`ru mark`); it drives
	// the row's glyph and color. Unset on group and job rows.
	attention protocol.Attention
	note      string
	// since is when the session entered that attention state, rendered as an age
	// on "needs you" rows: an agent that has been blocked for 20 minutes is a
	// different problem from one that just stopped.
	since time.Time
	// worktree/branch are set on a session row isolated in its own git worktree.
	worktree string
	branch   string
	// counts summarizes the row's subtree (sessions under a group, jobs under a
	// session) for the right-aligned chip.
	running, queued, failed, total int
}

// treePaneWidth is the sidebar's fixed inner width (columns), plus a divider.
const treePaneWidth = 26

func groupKey(g string) string      { return "g\x00" + g }
func sessionKey(g, s string) string { return "s\x00" + g + "\x00" + s }

// buildTreeRows flattens the group→session→job hierarchy into visible rows,
// skipping the children of any collapsed node.
func (m model) buildTreeRows() []treeRow {
	col := m.jobs.tree.collapsed
	var rows []treeRow
	for _, node := range m.nodes {
		gk := groupKey(node.group)
		grpRow := treeRow{kind: treeGroup, group: node.group, key: gk, hasKids: len(node.sessions) > 0}
		grpRow.total = len(node.sessions)
		for _, s := range node.sessions {
			r, q, _, f := jobStateCounts(node.jobs[s.Name])
			grpRow.running += r
			grpRow.queued += q
			grpRow.failed += f
		}
		rows = append(rows, grpRow)
		if col[gk] {
			continue
		}
		for _, s := range node.sessions {
			sk := sessionKey(node.group, s.Name)
			js := node.jobs[s.Name]
			running, queued, _, failed := jobStateCounts(js)
			rows = append(rows, treeRow{
				kind:      treeSession,
				group:     node.group,
				session:   s.Name,
				key:       sk,
				hasKids:   len(js) > 0,
				attention: s.Attention,
				note:      s.AttentionNote,
				since:     s.AttentionSince,
				worktree:  s.Worktree,
				branch:    s.Branch,
				running:   running,
				queued:    queued,
				failed:    failed,
				total:     len(js),
			})
			if col[sk] {
				continue
			}
			for _, j := range js {
				rows = append(rows, treeRow{kind: treeJob, group: node.group, session: s.Name, job: j})
			}
		}
	}
	return rows
}

// refreshTreeRows rebuilds the visible rows and keeps the cursor in range. Safe
// to call every tick; fold state lives in collapsed and is preserved.
func (m *model) refreshTreeRows() {
	if m.jobs.tree.collapsed == nil {
		m.jobs.tree.collapsed = map[string]bool{}
	}
	m.jobs.tree.rows = m.buildTreeRows()
	if m.jobs.tree.cursor >= len(m.jobs.tree.rows) {
		m.jobs.tree.cursor = len(m.jobs.tree.rows) - 1
	}
	if m.jobs.tree.cursor < 0 {
		m.jobs.tree.cursor = 0
	}
}

func (t *treePane) current() (treeRow, bool) {
	if t.cursor >= 0 && t.cursor < len(t.rows) {
		return t.rows[t.cursor], true
	}
	return treeRow{}, false
}

// --- key handling ---------------------------------------------------------

// toggleTree shows/hides the sidebar. Showing it also focuses it — opening the
// tree means you want to drive it. Hiding it drops that focus so the list
// regains navigation. Bound to the `,n` leader chord in handleJobsKey.
func (m model) toggleTree() (tea.Model, tea.Cmd) {
	m.jobs.tree.show = !m.jobs.tree.show
	if m.jobs.tree.show {
		m.jobs.tree.focus = true
		(&m).refreshTreeRows()
		m.jobs.tree.focus = true
	} else {
		m.jobs.tree.focus = false
		m.jobs.tree.zoom = false
		m.jobs.tree.menu = treeMenu{}
	}
	return m, nil
}

// handleTreeKey processes sidebar keys. `tab` moves focus between the tree and
// the list; while the tree has focus it owns navigation and fold keys. The bool
// reports whether the key was consumed (false lets the list handler run).
func (m model) handleTreeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "tab":
		if m.jobs.tree.show {
			m.jobs.tree.focus = !m.jobs.tree.focus
			return m, nil, true
		}
		return m, nil, false
	}

	if !m.jobs.tree.show || !m.jobs.tree.focus {
		return m, nil, false
	}

	// The action menu, when open, owns every key until it closes.
	if m.jobs.tree.menu.active {
		return m.handleTreeMenuKey(msg)
	}

	t := &m.jobs.tree
	switch msg.String() {
	case "esc":
		if t.zoom {
			t.zoom = false
			return m, nil, true
		}
		t.focus = false
	case "j", "down":
		if t.cursor < len(t.rows)-1 {
			t.cursor++
		}
	case "k", "up":
		if t.cursor > 0 {
			t.cursor--
		}
	case "g":
		t.cursor = 0
	case "G":
		if len(t.rows) > 0 {
			t.cursor = len(t.rows) - 1
		}
	case "l", "o", " ":
		(&m).treeToggleFold()
	case "h":
		(&m).treeCollapseOrParent()
	case "A":
		// NerdTree-style zoom: expand the pane to fill the whole box.
		t.zoom = !t.zoom
	case "m":
		// NerdTree-style action menu for the node under the cursor.
		return m.openTreeMenu()
	case "enter":
		return m.treeActivate()
	case "e":
		return m.treeEdit()
	case "n", "a":
		grp := ""
		if r, ok := t.current(); ok {
			grp = r.group
		}
		return m.openSessionForm(true, "", grp), textinput.Blink, true
	case "d":
		(&m).treeDelete()
	}
	return m, nil, true
}

// openTreeMenu builds the NerdTree-style action menu for the node under the
// cursor. The offered actions depend on whether it's a group, session, or job.
func (m model) openTreeMenu() (tea.Model, tea.Cmd, bool) {
	r, ok := m.jobs.tree.current()
	if !ok {
		return m, nil, true
	}
	var items []treeMenuItem
	switch r.kind {
	case treeGroup:
		items = []treeMenuItem{
			{"a", "add session", menuAdd},
			{"r", "rename group", menuRename},
			{"d", "delete group", menuDelete},
		}
	case treeSession:
		items = []treeMenuItem{
			{"o", "open session", menuOpen},
			{"a", "add session", menuAdd},
			{"r", "rename session", menuRename},
			{"d", "delete session", menuDelete},
		}
	case treeJob:
		items = []treeMenuItem{
			{"o", "open output", menuOpen},
			{"a", "add session", menuAdd},
		}
	}
	m.jobs.tree.menu = treeMenu{active: true, row: r, items: items}
	return m, nil, true
}

// handleTreeMenuKey drives the open action menu: j/k move, enter or a shortcut
// runs an item, esc/q/m closes it.
func (m model) handleTreeMenuKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	menu := &m.jobs.tree.menu
	switch msg.String() {
	case "esc", "q", "m":
		menu.active = false
		return m, nil, true
	case "j", "down":
		if menu.cursor < len(menu.items)-1 {
			menu.cursor++
		}
		return m, nil, true
	case "k", "up":
		if menu.cursor > 0 {
			menu.cursor--
		}
		return m, nil, true
	case "enter":
		if menu.cursor < len(menu.items) {
			return m.runTreeMenu(menu.items[menu.cursor].act)
		}
		return m, nil, true
	}
	for _, it := range menu.items {
		if msg.String() == it.key {
			return m.runTreeMenu(it.act)
		}
	}
	return m, nil, true
}

// runTreeMenu closes the menu and dispatches the chosen action. The menu opened
// on the current row and the cursor can't move while it's up, so the existing
// current()-based helpers act on the right node.
func (m model) runTreeMenu(act treeMenuAct) (tea.Model, tea.Cmd, bool) {
	m.jobs.tree.menu.active = false
	switch act {
	case menuOpen:
		return m.treeActivate()
	case menuAdd:
		grp := ""
		if r, ok := m.jobs.tree.current(); ok {
			grp = r.group
		}
		return m.openSessionForm(true, "", grp), textinput.Blink, true
	case menuRename:
		return m.treeEdit()
	case menuDelete:
		(&m).treeDelete()
	}
	return m, nil, true
}

// treeToggleFold folds/unfolds the group or session under the cursor.
func (m *model) treeToggleFold() {
	r, ok := m.jobs.tree.current()
	if !ok || r.key == "" || !r.hasKids {
		return
	}
	if m.jobs.tree.collapsed == nil {
		m.jobs.tree.collapsed = map[string]bool{}
	}
	m.jobs.tree.collapsed[r.key] = !m.jobs.tree.collapsed[r.key]
	m.refreshTreeRows()
}

// treeCollapseOrParent folds an expanded node, else jumps to its parent — the
// familiar `h` behavior in a tree.
func (m *model) treeCollapseOrParent() {
	r, ok := m.jobs.tree.current()
	if !ok {
		return
	}
	if r.key != "" && r.hasKids && !m.jobs.tree.collapsed[r.key] {
		m.jobs.tree.collapsed[r.key] = true
		m.refreshTreeRows()
		return
	}
	// Move up to the nearest shallower row (session→group, job→session).
	want := treeGroup
	if r.kind == treeJob {
		want = treeSession
	} else if r.kind == treeSession {
		want = treeGroup
	} else {
		return
	}
	for i := m.jobs.tree.cursor - 1; i >= 0; i-- {
		if m.jobs.tree.rows[i].kind == want {
			m.jobs.tree.cursor = i
			return
		}
	}
}

// treeActivate handles Enter: fold a group, open a session, page a job's output.
func (m model) treeActivate() (tea.Model, tea.Cmd, bool) {
	r, ok := m.jobs.tree.current()
	if !ok {
		return m, nil, true
	}
	switch r.kind {
	case treeGroup:
		(&m).treeToggleFold()
		return m, nil, true
	case treeSession:
		nm, cmd := m.openSession(r.session)
		return nm, cmd, true
	case treeJob:
		return m, openPagerCmd(r.job), true
	}
	return m, nil, true
}

// treeEdit opens the rename/edit form for the group or session under the cursor.
func (m model) treeEdit() (tea.Model, tea.Cmd, bool) {
	r, ok := m.jobs.tree.current()
	if !ok {
		return m, nil, true
	}
	switch r.kind {
	case treeGroup:
		return m.openGroupRenameForm(r.group), textinput.Blink, true
	case treeSession, treeJob:
		return m.openSessionForm(false, r.session, r.group), textinput.Blink, true
	}
	return m, nil, true
}

// treeDelete asks to delete the group or session under the cursor (the default
// session can't be deleted; the daemon rejects it).
func (m *model) treeDelete() {
	r, ok := m.jobs.tree.current()
	if !ok {
		return
	}
	switch r.kind {
	case treeGroup:
		m.jobs.confirm = confirmState{kind: confirmDeleteGroup, group: r.group}
	case treeSession:
		m.jobs.confirm = confirmState{kind: confirmDeleteSession, session: r.session}
	}
}

// --- rendering ------------------------------------------------------------

// treePaneLines renders the sidebar column as exactly h lines, width columns
// wide, scrolled to keep the cursor visible. The width varies: the fixed
// sidebar width normally, or the whole box when zoomed (`A`).
func (m model) treePaneLines(h, width int) []string {
	t := m.jobs.tree
	off := 0
	if t.cursor >= h {
		off = t.cursor - h + 1
	}
	out := make([]string, 0, h)
	for i := 0; i < h; i++ {
		idx := off + i
		if idx >= len(t.rows) {
			// Blank rows stay the terminal's own background: the sidebar is
			// separated by its divider and its cursor bar, not by a slab of fill.
			out = append(out, strings.Repeat(" ", width))
			continue
		}
		out = append(out, m.renderTreeRow(t.rows[idx], idx == t.cursor, width))
	}
	return out
}

// renderTreeRow draws one sidebar row as [cursor gutter][label …][chip]. The
// label is the hierarchy; the right-hand chip is the row's status — how long an
// agent has been waiting, or what its jobs are doing — so the eye can scan one
// column for "what needs me" instead of reading every name.
func (m model) renderTreeRow(r treeRow, selected bool, width int) string {
	var text, chip string
	style := treeSummaryStyle
	chipStyle := treeSummaryStyle
	switch r.kind {
	case treeGroup:
		text = foldMark(m.jobs.tree.collapsed[r.key], r.hasKids) + " " + strings.ToUpper(r.group)
		style = groupStyle
		chip, chipStyle = treeCountChip(r)
	case treeSession:
		// The attention glyph takes the first of the two indent columns, so rows
		// with and without one still line up under the group above them.
		glyph, attnStyle, reported := attentionMark(r.attention)
		text = glyph + " " + foldMark(m.jobs.tree.collapsed[r.key], r.hasKids) + " " + r.session
		if r.worktree != "" {
			// Mark isolation without spending the width a full branch name
			// needs; the branch itself shows in the pane status bar and the
			// tree's action menu.
			text += " ⊕"
		}
		style = sessionStyle
		if reported {
			style = attnStyle
		}
		// An unanswered agent reports its age, which outranks job counts: it is
		// the number the sidebar exists to show.
		if r.attention.NeedsYou() && !r.since.IsZero() {
			chip, chipStyle = waitAge(r.since), attnStyle
		} else {
			chip, chipStyle = treeCountChip(r)
		}
	case treeJob:
		label := r.job.Label
		if label == "" {
			label = r.job.Command
		}
		// No state glyph here: the sidebar is for the hierarchy, and a column of
		// markers next to every job competes with the one marker that matters —
		// the session's attention. Color alone carries the state; the table on
		// the right spells it out.
		text = fmt.Sprintf("    #%d %s", r.job.ID, label)
		style = jobRowStyle(r.job)
	}

	// Column 0 is a gutter holding the cursor bar. A background tint alone is
	// not enough to find the cursor with: it competes with the row's own color,
	// and a terminal that ignores background colors (or a light theme on a dark
	// terminal) renders it as nothing at all. A solid glyph always shows.
	gutter, gutterStyle := " ", lipgloss.NewStyle()
	switch {
	case selected && m.jobs.tree.focus:
		gutter, gutterStyle = "▌", treeCursorBarStyle
		style = style.Bold(true)
	case selected:
		gutter, gutterStyle = "▏", treeCursorDimBarStyle
	}

	body := padBetween(stripAnsi(text), stripAnsi(chip), width-1)
	if selected && m.jobs.tree.focus {
		return gutterStyle.Render(gutter) +
			style.Background(cRowFocusBg).Render(body.left) +
			chipStyle.Background(cRowFocusBg).Render(body.right)
	}
	if selected {
		// Unfocused cursor: the bar stays, in a quieter color, so the position
		// is still findable while the list has the keyboard.
		return gutterStyle.Render(gutter) +
			treeCursorDimStyle.Render(body.left+body.right)
	}
	return gutterStyle.Render(gutter) +
		style.Render(body.left) + chipStyle.Render(body.right)
}

// splitRow is a row cut into its label part and its right-aligned chip, each
// already padded, so a caller can style the two halves differently while the
// pair still occupies exactly the requested width.
type splitRow struct{ left, right string }

// padBetween lays out left … right in exactly width columns. The label is
// truncated before the chip is: a chip that gets cut loses its meaning
// entirely, whereas a clipped name is still recognizable.
func padBetween(left, right string, width int) splitRow {
	if width <= 0 {
		return splitRow{}
	}
	rw := lipgloss.Width(right)
	if rw > 0 {
		right = " " + right
		rw++
	}
	if rw >= width {
		return splitRow{right: fitToWidth(right, width)}
	}
	return splitRow{left: fitToWidth(left, width-rw), right: right}
}

// treeCountChip summarizes a row's jobs: failures first (the one count worth
// interrupting for), then activity, then a plain total. The suffix letters are
// the state names' initials, matching the table's uppercase labels. Quiet rows
// get no chip at all — an empty column is the fastest way to say "nothing here".
func treeCountChip(r treeRow) (string, lipgloss.Style) {
	switch {
	case r.failed > 0:
		return fmt.Sprintf("%dF", r.failed), finishedErrStyle
	case r.running > 0 && r.queued > 0:
		return fmt.Sprintf("%dR %dQ", r.running, r.queued), runningStyle
	case r.running > 0:
		return fmt.Sprintf("%dR", r.running), runningStyle
	case r.queued > 0:
		return fmt.Sprintf("%dQ", r.queued), queuedStyle
	case r.kind == treeGroup && r.total > 0:
		return fmt.Sprintf("%d", r.total), treeSummaryStyle
	default:
		return "", treeSummaryStyle
	}
}

// jobRowStyle colors a job row in the sidebar by its state, which is how the
// state reads there — the table on the right carries the labelled badge.
func jobRowStyle(j protocol.JobInfo) lipgloss.Style {
	switch j.State {
	case protocol.StateRunning:
		return runningStyle
	case protocol.StateQueued, protocol.StateAllocating:
		return queuedStyle
	case protocol.StateSkipped:
		return skippedStyle
	default:
		if !j.Result.Canceled && (j.Result.DiedBySignal || j.Result.ExitCode != 0) {
			return finishedErrStyle
		}
		return finishedStyle
	}
}

// waitAge renders how long a session has been in its current attention state,
// in the two characters a sidebar can spare: 45s, 12m, 3h, 2d.
func waitAge(since time.Time) string {
	d := time.Since(since)
	switch {
	case d < 0:
		return "now"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// treeMenuLines renders the NerdTree-style action menu as a centered box,
// listing each action with its shortcut and highlighting the cursor row.
func (m model) treeMenuLines(bodyH, inner int) []string {
	menu := m.jobs.tree.menu
	boxInner := modalInnerWidth(inner, 34)
	content := []string{
		treeSummaryStyle.Render(fitToWidth(" "+treeMenuTarget(menu.row), boxInner)),
		"",
	}
	for i, it := range menu.items {
		line := fmt.Sprintf(" (%s) %s", it.key, it.label)
		if i == menu.cursor {
			content = append(content, modalActiveStyle.Render(fitToWidth("›"+line, boxInner)))
		} else {
			content = append(content, treeSummaryStyle.Render(fitToWidth(" "+line, boxInner)))
		}
	}
	content = append(content, "", treeEmptyStyle.Render(fitToWidth("  j/k move · ⏎ run · esc close", boxInner)))
	return centerBox(modalBox("Menu", content, boxInner), bodyH, inner)
}

// treeMenuTarget names the node an open menu acts on, for its heading.
func treeMenuTarget(r treeRow) string {
	switch r.kind {
	case treeGroup:
		return "group " + r.group
	case treeSession:
		if r.branch != "" {
			return "session " + r.session + " ⊕ " + r.branch
		}
		return "session " + r.session
	case treeJob:
		label := r.job.Label
		if label == "" {
			label = r.job.Command
		}
		return fmt.Sprintf("job #%d %s", r.job.ID, label)
	}
	return ""
}

// foldMark returns the disclosure triangle for a node (blank when it has no
// children, so leaves don't pretend to be foldable).
func foldMark(collapsed, hasKids bool) string {
	if !hasKids {
		return " "
	}
	if collapsed {
		return "▸"
	}
	return "▾"
}

// treeHeaderCell renders the sidebar's column header in the same style as the
// job table header, so the whole top row reads as one unit: bright when the
// tree is focused, muted-but-legible otherwise. It carries the inbox count —
// how many sessions are waiting on you — because that number is the reason to
// look at the sidebar at all.
func (m model) treeHeaderCell(width int) string {
	s := jobsHeaderStyle
	if !m.jobs.tree.focus {
		s = jobsHeaderStyle.Foreground(cFgMuted)
	}
	needYou := 0
	for _, n := range m.nodes {
		for _, sess := range n.sessions {
			if sess.Attention.NeedsYou() {
				needYou++
			}
		}
	}
	chip := ""
	if needYou > 0 {
		chip = fmt.Sprintf("%d!", needYou)
	}
	body := padBetween(" SESSIONS", chip, width)
	if needYou > 0 {
		return s.Render(body.left) + jobsHeaderStyle.Foreground(cAmber).Render(body.right)
	}
	return s.Render(body.left + body.right)
}

func treeDividerCell() string { return borderStyle.Render("│") }
