package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/han/runic/internal/protocol"
)

// Key and mouse dispatch for the job-management screen, plus the cursor,
// selection and action helpers the bindings drive.

func (m model) handleJobsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.jobs.mode == jobsPager {
		return m.handlePagerKey(msg)
	}

	if m.jobs.form.active {
		return m.handleSessionFormKey(msg)
	}

	if m.jobs.settings.active {
		return m.handleSettingsFormKey(msg)
	}

	if m.jobs.picker.active {
		return m.handlePickerKey(msg)
	}

	if m.jobs.commanding {
		return m.handleJobsCommandKey(msg)
	}

	if m.jobs.helping {
		// The overlay is a page you read, not a flash card: it is longer than
		// any terminal, so it has to scroll, and a stray keystroke must not
		// throw it away mid-read. Esc and q close it, as everywhere else.
		switch msg.String() {
		case "esc", "q", "?":
			m.jobs.helping = false
			m.jobs.help = helpView{}
		case "j", "down":
			(&m).helpScroll(1)
		case "k", "up":
			(&m).helpScroll(-1)
		case "ctrl+d", "pgdown", " ":
			(&m).helpScroll(max(1, m.jobsBodyHeight()/2))
		case "ctrl+u", "pgup":
			(&m).helpScroll(-max(1, m.jobsBodyHeight()/2))
		case "g", "home":
			m.jobs.help.offset = 0
		case "G", "end":
			// Clamped down to the real end by helpViewport, which is the only
			// place the content length is known.
			m.jobs.help.offset = 1 << 20
		}
		return m, nil
	}

	if m.jobs.confirm.kind != confirmNone {
		c := m.jobs.confirm
		m.jobs.confirm = confirmState{}
		if s := msg.String(); s == "y" || s == "Y" {
			switch c.kind {
			case confirmRemove:
				return m, removeJobs(c.ids)
			case confirmClear:
				return m, clearFinishedCmd()
			case confirmDeleteSession:
				return m, deleteSession(c.session)
			case confirmDiscardSession:
				return m, discardSession(c.session)
			case confirmDeleteGroup:
				return m, deleteGroup(c.group)
			case confirmReset:
				return m, resetServerCmd()
			}
		}
		if c.kind == confirmDiscardSession {
			// Declining is a decision worth confirming back: the session is still
			// there, and so is the work that stopped it from going away.
			m.status = fmt.Sprintf("kept session %q — its worktree still has uncommitted work", c.session)
		}
		return m, nil
	}

	if m.jobs.filtering {
		switch msg.String() {
		case "enter":
			m.jobs.filtering = false
			m.jobs.filter = m.textInput.Value()
			m.textInput.Blur()
			m.refreshJobsRows()
			return m, nil
		case "esc":
			m.jobs.filtering = false
			m.textInput.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		m.jobs.filter = m.textInput.Value()
		m.refreshJobsRows()
		return m, cmd
	}

	key := msg.String()
	if !m.jobs.tree.menu.active {
		if key == "[" {
			m.cycleJobsSession(-1)
			return m, nil
		}
		if key == "]" {
			m.cycleJobsSession(1)
			return m, nil
		}
	}

	// Ctrl+W chord: vim-style focus movement between the tree sidebar (left) and
	// the job list (right), mirroring the split-view pane nav.
	if m.ctrlWPressed {
		m.ctrlWPressed = false
		switch key {
		case "h", "left", "ctrl+h", "backspace":
			if m.jobs.tree.show {
				m.jobs.tree.focus = true
			}
			return m, nil
		case "l", "right", "ctrl+l":
			m.jobs.tree.focus = false
			return m, nil
		case "w", "ctrl+w":
			if m.jobs.tree.show {
				m.jobs.tree.focus = !m.jobs.tree.focus
			}
			return m, nil
		}
		// unrecognized chord key: fall through and process it normally
	}
	if msg.Type == tea.KeyCtrlW {
		m.ctrlWPressed = true
		m.ctrlWTimer++
		id := m.ctrlWTimer
		return m, tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg { return ctrlWTimeoutMsg{id: id} })
	}

	// `,` leader chord. Handled before the tree so it works from anywhere,
	// including while the tree has focus. `,n` toggles the sidebar.
	if m.jobs.pending == ',' {
		m.jobs.pending = 0
		if key == "n" {
			return m.toggleTree()
		}
		// unknown leader key: fall through and process it normally
	} else if key == "," {
		return m.startPending(',')
	}

	// Tab / Shift+Tab roll the focused pane through its own views: the sidebar
	// between Spaces and Agents, the table through All, Running, … Binding the
	// key to the focused pane keeps one gesture for "next view" on a screen
	// where both panes have tabs. ←/→ do the same, but only from the table —
	// in the sidebar they stay fold keys.
	if !m.jobs.tree.menu.active {
		treeFocused := m.jobs.tree.show && m.jobs.tree.focus
		switch {
		case key == "tab":
			if treeFocused {
				(&m).cycleSideTab(1)
			} else {
				(&m).cycleTab(1)
			}
			return m, nil
		case key == "shift+tab":
			if treeFocused {
				(&m).cycleSideTab(-1)
			} else {
				(&m).cycleTab(-1)
			}
			return m, nil
		case key == "right" && !treeFocused:
			(&m).cycleTab(1)
			return m, nil
		case key == "left" && !treeFocused:
			(&m).cycleTab(-1)
			return m, nil
		}
	}

	// Tree sidebar: while it has focus it owns navigation and fold keys (else
	// the list handler below runs).
	if nm, cmd, done := m.handleTreeKey(msg); done {
		return nm, cmd
	}

	// Complete a pending two-key motion (gg jumps to top; sg/si/ss/st sort).
	if m.jobs.pending != 0 {
		op := m.jobs.pending
		m.jobs.pending = 0
		if op == 'g' && key == "g" {
			(&m).jobsGoto(0)
			return m, nil
		}
		if op == 's' {
			if mode, ok := chordSortMode(key); ok {
				(&m).applySort(mode)
				return m, nil
			}
		}
		// not a completion — fall through and process key normally
	}

	// Open the `:` command line. A colon typed or pasted with following
	// characters (e.g. ":config") arrives as one runes message, so seed the
	// input with whatever trails the colon.
	if strings.HasPrefix(key, ":") {
		m.jobs.commanding = true
		m.cmdInput.SetValue(key[1:])
		m.cmdInput.Focus()
		m.cmdInput.CursorEnd()
		return m, textinput.Blink
	}

	switch key {
	case "esc":
		if m.jobs.visual {
			m.jobs.visual = false
			return m, nil
		}
		if len(m.jobs.tagged) > 0 {
			m.jobs.tagged = nil
			return m, nil
		}
		return m.leaveManagement()
	case "q":
		// Always quit, even with a session open in the background — daemon-side
		// shells persist, and `esc` already covers "back to the session". Without
		// this, `q` after a detach just bounced back into the session.
		return m, tea.Quit
	case " ":
		// ranger/lf-style tagging: toggle the cursor job's selection, then
		// advance one row. Esc clears; actions consume the tagged set.
		if r, ok := m.jobsCursorRow(); ok {
			if m.jobs.tagged == nil {
				m.jobs.tagged = make(map[int]bool)
			}
			if m.jobs.tagged[r.job.ID] {
				delete(m.jobs.tagged, r.job.ID)
			} else {
				m.jobs.tagged[r.job.ID] = true
			}
		}
		(&m).jobsMove(1)
	case "j", "down":
		(&m).jobsMove(1)
	case "k", "up":
		(&m).jobsMove(-1)
	case "ctrl+d":
		(&m).jobsMove(m.jobsBodyHeight() / 2)
	case "ctrl+u":
		(&m).jobsMove(-m.jobsBodyHeight() / 2)
	case "G":
		(&m).jobsGoto(len(m.jobs.rows) - 1)
	case "g":
		return m.startPending('g')
	case "V":
		if m.jobs.visual {
			m.jobs.visual = false
		} else if _, ok := m.jobsSelected(); ok {
			m.jobs.visual = true
			m.jobs.anchor = m.jobs.cursor
		}
	case "/":
		m.jobs.filtering = true
		m.textInput.SetValue(m.jobs.filter)
		m.textInput.Focus()
		return m, textinput.Blink
	case "?":
		m.jobs.helping = true
		return m, nil
	case "S":
		// Open the session picker (reaches every session, incl. empty ones).
		return m.openSessionPicker(), nil
	case "s":
		// Start a sort chord: sg / si / ss / st pick the field, repeating the
		// active field's chord reverses it. Header shows column + direction.
		return m.startPending('s')
	case "R":
		// Reverse the current sort (bada-style flip).
		m.jobs.sortRev = !m.jobs.sortRev
		m.refreshJobsRows()
		return m, nil
	case "e":
		// Edit the cursor job in one box: job name + session + group. (Sessions
		// and groups are edited from the tree sidebar.)
		if r, ok := m.jobsCursorRow(); ok {
			return m.openJobEditForm(r.job, r.group), textinput.Blink
		}
	case "n", "a":
		// Create a new session via the edit box (blank form).
		return m.openSessionForm(true, "", ""), textinput.Blink
	case "x":
		if ids := m.jobsActionIDs(); len(ids) > 0 {
			m.jobs.visual = false
			m.jobs.tagged = nil
			return m, killJobs(ids)
		}
	case "u":
		if ids := m.jobsActionIDs(); len(ids) > 0 {
			m.jobs.visual = false
			m.jobs.tagged = nil
			return m, makeUrgentJobs(ids)
		}
	case "r":
		if ids := m.jobsActionIDs(); len(ids) > 0 {
			m.jobs.visual = false
			m.jobs.tagged = nil
			return m, rerunJobs(ids)
		}
	case "d":
		// Remove jobs (finished ones without a prompt).
		ids := m.jobsActionIDs()
		if len(ids) == 0 {
			return m, nil
		}
		allFinished := m.actionAllFinished()
		m.jobs.visual = false
		m.jobs.tagged = nil
		if allFinished {
			return m, removeJobs(ids)
		}
		m.jobs.confirm = confirmState{kind: confirmRemove, ids: ids}
		return m, nil
	case "D":
		// Delete every finished job at once. They are inert, so skip the
		// confirmation prompt (`C` keeps the confirming variant).
		return m, clearFinishedCmd()
	case "C":
		m.jobs.confirm = confirmState{kind: confirmClear}
	case "enter":
		return m.activateRow()
	case "o":
		if j, ok := m.jobsSelected(); ok {
			return m, openPagerCmd(j)
		}
	}
	return m, nil
}

// mgmtBodyTopLine is the terminal row (0-based) where the first list row is
// drawn: after the Jobs pane's top border, the tab strip and the column header.
func (m model) mgmtBodyTopLine() int { return 3 }

// mgmtTabStripLine is the terminal row holding the view tabs.
const mgmtTabStripLine = 1

// handleMouse maps clicks and wheel scrolls on the management screen to row
// selection/activation. Mouse events are only captured while this screen shows.
func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.viewMode == viewSplit {
		return m.handleSessionMouse(msg)
	}
	if m.viewMode != viewJobs || m.jobs.mode != jobsTable || m.jobs.form.active ||
		m.jobs.settings.active || m.jobs.filtering {
		return m, nil
	}
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		if m.jobs.helping {
			(&m).helpScroll(-1)
			return m, nil
		}
		(&m).jobsMove(-1)
		return m, nil
	case tea.MouseButtonWheelDown:
		if m.jobs.helping {
			(&m).helpScroll(1)
			return m, nil
		}
		(&m).jobsMove(1)
		return m, nil
	case tea.MouseButtonLeft:
		if m.jobsOverlay() {
			return m, nil
		}
		l := m.jobsLayout()
		// The hint bar sits directly under the panes and lists the keys that
		// apply right now, so it doubles as this screen's toolbar.
		if msg.Y == l.paneH {
			if _, spans := m.jobsFooterSpans(m.width); spans != nil {
				if h, ok := hintAt(spans, msg.X); ok {
					if nm, cmd, handled := m.pressHint(h); handled {
						return nm, cmd
					}
				}
			}
			return m, nil
		}
		// A click inside the Sessions pane: its tab strip switches the view,
		// its rows take focus and move the cursor — the same two gestures the
		// Jobs pane answers to.
		if l.showTree && msg.X < l.treeW {
			return m.clickTree(msg, l)
		}
		if l.zoom || msg.X <= l.listX || msg.X >= l.listX+l.listW-1 {
			return m, nil
		}
		if msg.Y == 0 {
			_, spans := m.jobsSessionStrip(l.listW - 2)
			for _, sp := range spans {
				if msg.X-l.listX-1 >= sp.start && msg.X-l.listX-1 < sp.end {
					m.selectJobsSession(sp.session)
					break
				}
			}
			return m, nil
		}
		if msg.Y == mgmtTabStripLine {
			// A click on a tab switches to it.
			_, spans := m.renderTabStrip(l.listW - 2)
			x := msg.X - l.listX - 1
			for _, sp := range spans {
				if x >= sp.start && x < sp.end {
					(&m).switchTab(sp.tab)
					break
				}
			}
			return m, nil
		}
		top := m.mgmtBodyTopLine()
		if msg.Y < top || msg.Y >= top+m.jobsBodyHeight() {
			return m, nil
		}
		// Clicking the table hands it focus, like clicking a pane.
		m.jobs.tree.focus = false
		idx := m.jobs.offset + (msg.Y - top)
		if idx < 0 || idx >= len(m.jobs.rows) {
			return m, nil
		}
		m.jobs.cursor = idx
		m.jobs.cursorKey = rowKey(m.jobs.rows[idx])
		m.jobs.offset = clampOffset(idx, m.jobs.offset, m.jobsBodyHeight(), len(m.jobs.rows))
		// Double-click opens, the way it does everywhere else. A single click
		// still only selects, so a stray one cannot launch anything.
		if (&m).doubleClick("job|" + rowKey(m.jobs.rows[idx])) {
			return m.activateRow()
		}
		return m, nil
	}
	return m, nil
}

// clickTree handles a left click inside the Sessions pane: the tab strip
// switches view, a tree row takes focus and moves the cursor there. Clicking a
// row only selects it — opening stays on Enter, so a stray click can't launch
// into a session.
func (m model) clickTree(msg tea.MouseMsg, l jobsLayout) (tea.Model, tea.Cmd) {
	inner := l.treeW - 2
	if msg.Y == mgmtTabStripLine {
		_, spans := m.renderSideTabStrip(inner)
		x := msg.X - 1 // the pane's left border
		for _, sp := range spans {
			if x >= sp.start && x < sp.end {
				m.jobs.tree.focus = true
				(&m).switchSideTab(sp.tab)
				break
			}
		}
		return m, nil
	}
	bodyH := treeBodyHeight(l.paneH)
	if msg.Y < treeBodyTopLine || msg.Y >= treeBodyTopLine+bodyH {
		return m, nil
	}
	idx := treeScrollOffset(m.jobs.tree.cursor, bodyH) + (msg.Y - treeBodyTopLine)
	if idx < 0 || idx >= len(m.jobs.tree.rows) {
		return m, nil
	}
	m.jobs.tree.focus = true
	m.jobs.tree.cursor = idx
	// Double-click opens what a single click selected: the standard gesture,
	// and the one people try first. The single-click guard stays, so a stray
	// click still cannot drop you into a session.
	if (&m).doubleClick(treeRowTarget(m.jobs.tree.rows[idx])) {
		nm, cmd, _ := m.treeActivate()
		return nm, cmd
	}
	return m, nil
}

// leaveManagement exits the management view: back to the active session's split
// view, or quits when launched with --jobs or when no session is open.
func (m model) leaveManagement() (tea.Model, tea.Cmd) {
	if m.jobsOnly || m.activeSession == "" || m.activeRoot() == nil {
		return m, tea.Quit
	}
	m.viewMode = viewSplit
	m.jobs.tree.focus = false
	m.refreshTreeRows()
	m.mouseOn = true
	return m, tea.Batch(clearScreenCmd(), tea.EnableMouseCellMotion)
}

// activateRow handles Enter on a job row: open its output pager (like `o`).
// Sessions are opened from the tree sidebar or the picker (`S`).
func (m model) activateRow() (tea.Model, tea.Cmd) {
	r, ok := m.jobsCursorRow()
	if !ok {
		return m, nil
	}
	return m, openPagerCmd(r.job)
}

// openSession leaves the management view and activates the given session,
// building or restoring its terminal panes.
func (m model) openSession(name string) (tea.Model, tea.Cmd) {
	return m.openSessionPane(name, "")
}

// openSessionPane is openSession with a target: a non-empty pane focuses that
// pane rather than the session's first leaf. Opening a blocked agent should
// land on the agent, not merely in its neighbourhood — the pane is the thing
// that is waiting on you.
func (m model) openSessionPane(name, pane string) (tea.Model, tea.Cmd) {
	if name == "" {
		return m, nil
	}
	m.viewMode = viewSplit
	m.jobs.tree.focus = false
	m.refreshTreeRows()
	m.mouseOn = true
	nm, cmd := m.activateSession(name)
	if pane != "" {
		(&nm).focusPaneNamed(pane)
	}
	return nm, tea.Batch(cmd, tea.EnableMouseCellMotion)
}

// chordSortMode maps the second key of an `s` sort chord to its field.
func chordSortMode(key string) (sortMode, bool) {
	switch key {
	case "g":
		return sortGrouped, true
	case "i":
		return sortByID, true
	case "s":
		return sortByState, true
	case "t":
		return sortByTime, true
	}
	return 0, false
}

// applySort selects a sort field; re-selecting the active field reverses it.
func (m *model) applySort(mode sortMode) {
	if m.jobs.sortMode == mode {
		m.jobs.sortRev = !m.jobs.sortRev
	} else {
		m.jobs.sortMode = mode
		m.jobs.sortRev = false
	}
	m.refreshJobsRows()
}

func (m model) startPending(op byte) (tea.Model, tea.Cmd) {
	m.jobs.pending = op
	m.jobs.pendingTimer++
	id := m.jobs.pendingTimer
	return m, tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg { return jobsGTimeoutMsg{id: id} })
}

func parseSortMode(s string) (sortMode, bool) {
	switch strings.ToLower(s) {
	case "group", "grouped":
		return sortGrouped, true
	case "id":
		return sortByID, true
	case "state":
		return sortByState, true
	case "time":
		return sortByTime, true
	}
	return sortGrouped, false
}

func (m *model) jobsGoto(idx int) {
	n := len(m.jobs.rows)
	if n == 0 {
		return
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	m.jobs.cursor = idx
	m.jobs.cursorKey = rowKey(m.jobs.rows[idx])
	m.jobs.offset = clampOffset(idx, m.jobs.offset, m.jobsBodyHeight(), n)
}

// jobsActionRows returns the job rows an action applies to: the visual
// selection when active (job rows only), otherwise just the cursor job row.
func (m model) jobsActionRows() []protocol.JobInfo {
	// Tagged (Space) selection wins; it survives cursor movement like ranger's.
	// Resolved against every known job, not just the visible rows: a tag that
	// scrolled out of view behind a filter is still part of the selection the
	// footer counts, and falling through to the cursor row here would act on a
	// job the user never picked. refreshJobsRows drops tags for dead jobs, so
	// everything still tagged exists.
	if len(m.jobs.tagged) > 0 {
		var rows []protocol.JobInfo
		for _, j := range m.jobs.allJobs {
			if m.jobs.tagged[j.ID] {
				rows = append(rows, j)
			}
		}
		return rows
	}
	if m.jobs.visual {
		lo, hi := m.visualRange()
		var rows []protocol.JobInfo
		for i := lo; i <= hi && i < len(m.jobs.rows); i++ {
			if i >= 0 {
				rows = append(rows, m.jobs.rows[i].job)
			}
		}
		return rows
	}
	if j, ok := m.jobsSelected(); ok {
		return []protocol.JobInfo{j}
	}
	return nil
}

// jobsActionIDs returns the job IDs an action applies to.
func (m model) jobsActionIDs() []int {
	rows := m.jobsActionRows()
	ids := make([]int, 0, len(rows))
	for _, j := range rows {
		ids = append(ids, j.ID)
	}
	return ids
}

// actionAllFinished reports whether every job in the current action set is
// finished — i.e. safe to remove without a confirmation prompt.
func (m model) actionAllFinished() bool {
	rows := m.jobsActionRows()
	if len(rows) == 0 {
		return false
	}
	for _, j := range rows {
		if j.State != protocol.StateFinished {
			return false
		}
	}
	return true
}

func (m model) visualRange() (int, int) {
	lo, hi := m.jobs.anchor, m.jobs.cursor
	if lo > hi {
		lo, hi = hi, lo
	}
	return lo, hi
}

func (m *model) jobsMove(delta int) {
	n := len(m.jobs.rows)
	if n == 0 {
		return
	}
	m.jobs.cursor += delta
	if m.jobs.cursor < 0 {
		m.jobs.cursor = 0
	}
	if m.jobs.cursor >= n {
		m.jobs.cursor = n - 1
	}
	m.jobs.cursorKey = rowKey(m.jobs.rows[m.jobs.cursor])
	m.jobs.offset = clampOffset(m.jobs.cursor, m.jobs.offset, m.jobsBodyHeight(), n)
}
