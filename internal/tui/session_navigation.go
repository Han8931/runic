package tui

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Session navigation uses the daemon's catalog, including sessions whose panes
// have not been opened by this client yet.
func (m model) sessionNames() []string {
	seen := map[string]bool{}
	for _, n := range m.nodes {
		for _, s := range n.sessions {
			seen[s.Name] = true
		}
	}
	if len(m.nodes) == 0 {
		for _, s := range m.openSessionTabs() {
			seen[s] = true
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// sessionStrip keeps the selected tab visible even with many or long names.
// Hidden neighbours are indicated by arrows and remain reachable by keyboard.
func (m model) sessionStrip(width int, names []string, active string) (string, []termTabSpan) {
	if width < 4 {
		return fitToWidth("", width), nil
	}
	selected := 0
	for i, name := range names {
		if name == active {
			selected = i
		}
	}
	labels := make([]string, len(names))
	for i, name := range names {
		label := name
		if name == "" {
			label = "All sessions"
		}
		glyph, st, reported := attentionMark(m.sessionAttention(name))
		label = strings.TrimRight(truncateToWidth(label, max(1, width-8)), " ")
		if reported {
			label = glyph + " " + label
		}
		if name == active {
			labels[i] = chipStyle.Render(" " + label + " ")
		} else {
			labels[i] = tabInactiveStyle.Render(" " + label + " ")
			if reported {
				labels[i] = st.Render(" " + label + " ")
			}
		}
	}
	start := selected
	used := 0
	if len(labels) > 0 {
		used = lipgloss.Width(labels[selected])
	}
	for start > 0 && used+lipgloss.Width(labels[start-1])+1 <= width-4 {
		start--
		used += lipgloss.Width(labels[start]) + 1
	}
	var b strings.Builder
	col := 0
	write := func(s string) { b.WriteString(s); col += lipgloss.Width(s) }
	if start > 0 {
		write(helpStyle.Render("‹ "))
	} else {
		write(" ")
	}
	var spans []termTabSpan
	for i := start; i < len(names); i++ {
		if col+lipgloss.Width(labels[i]) > width-2 {
			write(helpStyle.Render(" ›"))
			break
		}
		from := col
		write(labels[i])
		spans = append(spans, termTabSpan{session: names[i], start: from, end: col})
		write(" ")
	}
	return fitToWidth(b.String(), width), spans
}

func (m model) jobsSessionStrip(width int) (string, []termTabSpan) {
	return m.sessionStrip(width, append([]string{""}, m.sessionNames()...), m.jobs.session)
}

func (m *model) selectJobsSession(name string) {
	m.jobs.session = name
	m.jobs.tagged = nil
	m.jobs.visual = false
	m.jobs.offset = 0
	m.refreshJobsRows()
}

func (m *model) cycleJobsSession(delta int) {
	names := append([]string{""}, m.sessionNames()...)
	at := 0
	for i, name := range names {
		if name == m.jobs.session {
			at = i
		}
	}
	m.selectJobsSession(names[(at+delta+len(names))%len(names)])
}

func (m model) termSidebarWidth() int {
	if m.termSidebarHidden || m.width < 60 {
		return 0
	}
	return treeOuterW
}

func (m model) focusSessionSidebar() (model, tea.Cmd) {
	if m.width < 60 {
		m, cmd := m.openJobsView()
		m.jobs.tree.show = true
		m.jobs.tree.focus = true
		return m, cmd
	}
	m.termSidebarHidden = false
	m.jobs.tree.show = true
	m.jobs.tree.focus = !m.jobs.tree.focus
	m.refreshTreeRows()
	m.resizeShell()
	return m, clearScreenCmd()
}

func (m model) handleSessionSidebarKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.jobs.form.active || m.jobs.confirm.kind != confirmNone {
		return m.handleJobsKey(msg)
	}
	switch msg.String() {
	case "tab":
		m.cycleSideTab(1)
		return m, nil
	case "shift+tab":
		m.cycleSideTab(-1)
		return m, nil
	case "esc":
		if !m.jobs.tree.menu.active {
			m.jobs.tree.focus = false
			return m, nil
		}
	case "A":
		return m, nil // keep the shell visible
	}
	nm, cmd, _ := m.handleTreeKey(msg)
	return nm, cmd
}

func (m model) handleSessionMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress || m.inputMode != inputNone || m.jobs.form.active || m.jobs.confirm.kind != confirmNone || m.jobs.tree.menu.active {
		return m, nil
	}
	// The status bar's key caps are this view's toolbar — notably `^B d detach`,
	// which is otherwise reachable only by already knowing the chord. Checked
	// before the sidebar, which spans the rows above but would otherwise swallow
	// a click on the left half of the footer.
	if msg.Y == m.height-1 && m.inputMode == inputNone && msg.Button == tea.MouseButtonLeft {
		if _, spans := m.footerSpans(); spans != nil {
			if h, ok := hintAt(spans, msg.X); ok {
				if nm, cmd, handled := m.pressHint(h); handled {
					return nm, cmd
				}
			}
		}
		return m, nil
	}
	sw := m.termSidebarWidth()
	if sw > 0 && msg.X < sw {
		m.jobs.tree.show = true
		m.jobs.tree.focus = true
		if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
			delta := 1
			if msg.Button == tea.MouseButtonWheelUp {
				delta = -1
			}
			m.jobs.tree.cursor = max(0, min(len(m.jobs.tree.rows)-1, m.jobs.tree.cursor+delta))
			return m, nil
		}
		if msg.Button != tea.MouseButtonLeft {
			return m, nil
		}
		_, h := m.termRegionSize()
		h = m.sessionSidebarPaneHeight(h)
		nm, cmd := m.clickTree(msg, jobsLayout{treeW: sw, paneH: h})
		next := nm.(model)
		if msg.Y >= treeBodyTopLine && msg.Y < treeBodyTopLine+treeBodyHeight(h) {
			idx := treeScrollOffset(m.jobs.tree.cursor, treeBodyHeight(h)) + msg.Y - treeBodyTopLine
			if idx >= 0 && idx < len(m.jobs.tree.rows) {
				nm, cmd, _ = next.treeActivate()
			}
		}
		return nm, cmd
	}
	if msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	offset := 0
	if sw > 0 {
		offset = sw + paneGap
	}
	if msg.Y == 0 {
		w, _ := m.termRegionSize()
		_, spans := m.sessionStrip(w, m.sessionNames(), m.activeSession)
		for _, sp := range spans {
			if msg.X-offset >= sp.start && msg.X-offset < sp.end {
				return m.openSession(sp.session)
			}
		}
		return m, nil
	}
	// A click in the terminal focuses the corresponding split pane.
	w, h := m.termRegionSize()
	if root := m.activeRoot(); root != nil {
		leaves, _ := root.layout(rect{x: 0, y: 0, w: w - 2, h: h - 2})
		for _, leaf := range leaves {
			r := leaf.rect
			x, y := msg.X-offset-1, msg.Y-1
			if x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h {
				m.focusPane = leaf.node
				m.syncFocusShell()
				m.jobs.tree.focus = false
				break
			}
		}
	}
	return m, nil
}

// Keep cached live panes attached to the renamed session after a successful
// server rename, including when a subsequent move to another group fails.
func (m *model) renameLocalSession(oldName, newName string) {
	if root := m.layouts[oldName]; root != nil {
		delete(m.layouts, oldName)
		m.layouts[newName] = root
		for _, leaf := range root.leaves() {
			if leaf.shell != nil {
				leaf.shell.mu.Lock()
				leaf.shell.session = newName
				leaf.shell.mu.Unlock()
			}
		}
	}
	if m.activeSession == oldName {
		m.activeSession = newName
	}
	if m.jobs.session == oldName {
		m.jobs.session = newName
	}
}

func (m model) sessionSidebarPaneHeight(height int) int {
	if height >= 10 {
		return height - 2
	}
	return height
}

func (m model) renderSessionSidebar(width, height int) []string {
	inner := width - 2
	bodyH := treeBodyHeight(m.sessionSidebarPaneHeight(height))
	lines := append([]string{m.treeHeaderCell(inner)}, m.treePaneLines(bodyH, inner)...)
	if height >= 10 {
		state, note := "sessions", "n new · e edit/move"
		if r, ok := m.jobs.tree.current(); ok {
			if r.kind == treeSession || r.kind == treeAgent {
				state = agentStateWord(r.attention, r.seen)
				note = r.note
				if note == "" {
					note = r.branch
				}
				if note == "" {
					note = "enter open · e edit/move"
				}
			} else {
				state = r.group
			}
		}
		lines = append(lines, treeSummaryStyle.Render(fitToWidth(" "+state, inner)), helpStyle.Render(fitToWidth(" "+note, inner)))
	}
	return paneBox(lines, width, height, m.jobs.tree.focus)
}
