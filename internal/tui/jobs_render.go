package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/han/runic/internal/format"
	"github.com/han/runic/internal/protocol"
)

// Rendering for the management screen: the job table (columns, rows,
// header), the surrounding chrome (borders, detail pane, footer, HW bar),
// and the shared modal-box helpers.

// columnWidths is the job table's column budget. A width of 0 means the column
// is hidden at this terminal size; the row and header renderers skip it and its
// separator entirely, so nothing is left holding empty space.
//
// There is one WORK column rather than the old NAME + COMMAND pair. For an
// unnamed job those two held the same text, spending a third of the table to
// say one thing twice.
type columnWidths struct {
	id, group, session, state, tm, timeout, work int
}

const (
	colIDW    = 5
	colStateW = 12 // the widest badge: "× signal 15", "◆ retry 1/2"
	colTimeW  = 8
	colSessW  = 12
	colGroupW = 10
	colTmoW   = 8
	// colWorkWant is the width below which a command stops being readable. It
	// is a floor the optional columns respect, not a guarantee: a very narrow
	// terminal gives WORK whatever is left.
	colWorkWant = 24
	colIDGap    = 3 // three spaces after ID, one between every other column
)

// computeJobColumns budgets the table by usefulness instead of a fixed layout.
// What a job *is* (WORK), what it is doing (STATE) and how long it has been
// doing it (TIME) always survive; the scope columns are added only once those
// fit comfortably.
//
// The previous fixed budget reserved every column unconditionally and clawed
// space back from the text ones, which meant that below about 100 columns WORK
// collapsed to a single character — the table rendered without ever saying what
// any job was. At 80 columns it fell off the pane entirely.
//
// anyTimeout drops TIMEOUT when nothing in view has one, which is the common
// case since it is opt-in per job.
func computeJobColumns(inner int, anyTimeout bool) columnWidths {
	c := columnWidths{id: colIDW, state: colStateW, tm: colTimeW}
	// The base set is ID, STATE, TIME and WORK: the ID gap, plus a separator
	// between each of the three columns that follow it.
	used := c.id + colIDGap + c.state + c.tm + 2
	// Optional columns, least dispensable first. Each costs its width plus one
	// separator, and is only taken while WORK keeps its preferred floor.
	for _, o := range []struct {
		w     int
		into  *int
		avail bool
	}{
		{colSessW, &c.session, true},
		{colGroupW, &c.group, true},
		{colTmoW, &c.timeout, anyTimeout},
	} {
		if !o.avail || used+1+o.w+colWorkWant > inner {
			continue
		}
		*o.into = o.w
		used += 1 + o.w
	}
	c.work = max(1, inner-used)
	return c
}

// jobsAnyTimeout reports whether any job currently in the table has a timeout,
// which is what decides if the TIMEOUT column earns its space.
func (m model) jobsAnyTimeout() bool {
	for _, r := range m.jobs.rows {
		if r.job.TimeoutMS > 0 {
			return true
		}
	}
	return false
}

// joinCols lays out the visible cells in table order: three spaces after ID,
// one between the rest. Hidden columns (width 0) contribute nothing, not even
// a separator.
func joinCols(sep string, id string, rest []string) string {
	var b strings.Builder
	b.WriteString(id)
	b.WriteString(strings.Repeat(sep, colIDGap))
	for i, cell := range rest {
		if i > 0 {
			b.WriteString(sep)
		}
		b.WriteString(cell)
	}
	return b.String()
}

func formatJobRow(j protocol.JobInfo, group string, c columnWidths) string {
	return renderJobsRow(j, group, c, nil)
}

// renderJobsRow renders one table row. When bg is non-nil the row is drawn as a
// highlight: every cell — and the padding within it — carries that background,
// while each cell keeps its own semantic foreground. This lets the focused row
// stay tinted without flattening the state colors (running=green, failed=red).
// displaySession hides the implicit "default" session name in the flat table —
// a job with no explicit session reads as an empty SESSION cell, not "default".
func displaySession(name string) string {
	if name == "default" {
		return ""
	}
	return name
}

func renderJobsRow(j protocol.JobInfo, group string, c columnWidths, bg lipgloss.TerminalColor) string {
	timeStr := ""
	if j.State == protocol.StateRunning || j.State == protocol.StateFinished {
		timeStr = format.Duration(format.ElapsedMS(j))
	}
	stateTxt, stateStyle := jobStateText(j)
	st := func(s lipgloss.Style) lipgloss.Style {
		if bg != nil {
			return s.Background(bg)
		}
		return s
	}
	cell := func(text string, w int, style lipgloss.Style) string {
		return st(style).Render(fitToWidth(stripAnsi(text), w))
	}
	sep := " "
	if bg != nil {
		sep = st(lipgloss.NewStyle()).Render(" ")
	}
	timeoutTxt, timeoutStyle := "-", treeEmptyStyle
	if j.TimeoutMS > 0 {
		timeoutTxt, timeoutStyle = durationCompact(time.Duration(j.TimeoutMS)*time.Millisecond), queuedStyle
	}
	// Order matters: this is the table's left-to-right layout.
	var rest []string
	add := func(w int, s string) {
		if w > 0 {
			rest = append(rest, s)
		}
	}
	add(c.group, cell(group, c.group, groupStyle))
	add(c.session, cell(displaySession(j.Session), c.session, sessionStyle))
	add(c.state, cell(stateTxt, c.state, stateStyle))
	add(c.tm, cell(timeStr, c.tm, lipgloss.NewStyle().Foreground(cFg)))
	add(c.timeout, cell(timeoutTxt, c.timeout, timeoutStyle))
	add(c.work, workCell(j, c.work, st))
	return joinCols(sep, cell(fmt.Sprintf("%*d", c.id, j.ID), c.id, jobIDStyle), rest)
}

// workCell renders the WORK column: what the job is. A named job leads with its
// name and trails its command in a dim tone when both fit; an unnamed job shows
// the command once. The old layout put the command in COMMAND and *also* echoed
// it into NAME whenever the job had no label, so the two columns duplicated
// each other for exactly the jobs that needed the room most.
func workCell(j protocol.JobInfo, w int, st func(lipgloss.Style) lipgloss.Style) string {
	cmdStyle := lipgloss.NewStyle().Foreground(cFg)
	if j.Label == "" {
		return st(cmdStyle).Render(fitToWidth(stripAnsi(j.Command), w))
	}
	name := stripAnsi(j.Label)
	// Show the command alongside the name only when enough of it survives to
	// be worth reading; otherwise the name owns the column.
	if nameW := lipgloss.Width(name); nameW+2+6 <= w {
		return st(jobNameStyle).Render(name) +
			st(lipgloss.NewStyle()).Render("  ") +
			st(treeEmptyStyle).Render(fitToWidth(stripAnsi(j.Command), w-nameW-2))
	}
	return st(jobNameStyle).Render(fitToWidth(name, w))
}

// durationCompact renders a duration without trailing zero units (30m0s → 30m).
func durationCompact(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

// padRowBg extends a highlighted row's background to the full inner width. For
// unhighlighted rows (bg == nil) it is a no-op — the pane box pads those.
func padRowBg(s string, width int, bg lipgloss.TerminalColor) string {
	if bg == nil {
		return s
	}
	if w := lipgloss.Width(s); w < width {
		return s + lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", width-w))
	}
	return s
}

// clockOrEmpty renders a timestamp as HH:MM:SS, or blank if unset.
func clockOrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("15:04:05")
}

func (m model) jobsHeaderRow(c columnWidths) string {
	arrow := "▲"
	if m.jobs.sortRev {
		arrow = "▼"
	}
	// mark tags the header of the currently-sorted column with the arrow.
	mark := func(title string, active bool) string {
		if active {
			return title + " " + arrow
		}
		return title
	}
	// Plain text; the caller block-highlights the whole row with jobsHeaderStyle.
	// Hidden columns are skipped so the header matches the rows beneath it.
	var rest []string
	add := func(w int, title string) {
		if w > 0 {
			rest = append(rest, fitToWidth(title, w))
		}
	}
	add(c.group, mark("GROUP", m.jobs.sortMode == sortGrouped))
	add(c.session, "SESSION")
	add(c.state, mark("STATE", m.jobs.sortMode == sortByState))
	add(c.tm, mark("TIME", m.jobs.sortMode == sortByTime))
	add(c.timeout, "TIMEOUT")
	add(c.work, "WORK")
	return joinCols(" ", fmt.Sprintf("%*s", c.id, mark("ID", m.jobs.sortMode == sortByID)), rest)
}

// --- rendering ------------------------------------------------------------

func (m model) renderJobsView(w, h int) string {
	if m.jobs.mode == jobsPager {
		return m.renderPager(w, h)
	}
	if w < 20 || h < 6 {
		return strings.Repeat("\n", max(0, h-1))
	}
	l := m.jobsLayout()
	gap := strings.Repeat(" ", paneGap)

	var tree, list, detail []string
	if l.showTree {
		tree = m.renderTreePanel(l.treeW, l.paneH)
	}
	if !l.zoom {
		list = m.renderListPanel(l.listW, l.listH, l.overlay)
		if l.detailH > 0 {
			detail = m.renderDetailPanel(l.detailW, l.detailH)
		}
	}

	lines := make([]string, 0, h)
	for i := 0; i < l.paneH; i++ {
		var line string
		if l.showTree {
			line = tree[i]
			if l.zoom {
				lines = append(lines, line)
				continue
			}
			line += gap
		}
		switch {
		case i < l.listH:
			line += list[i]
		case i-l.listH < len(detail):
			line += detail[i-l.listH]
		}
		lines = append(lines, line)
	}

	lines = append(lines, m.jobsFooterLine(w))
	if m.jobs.commanding {
		// Candidates from an ambiguous tab, so completion is a visible choice
		// rather than a blind rotation.
		lines = append(lines, m.completionLines(w)...)
	}
	lines = append(lines, m.jobsStatusBar(w))
	return joinExact(lines, w, h)
}

// paneBox frames lines in a rounded border, gorae-style. The focused pane draws
// its frame in the accent so focus reads from across the screen.
func paneBox(lines []string, w, h int, focused bool) []string {
	st := borderStyle
	tl, tr, bl, br, horizontal, vertical := "╭", "╮", "╰", "╯", "─", "│"
	if focused {
		st = focusBorderStyle
		tl, tr, bl, br, horizontal, vertical = "┏", "┓", "┗", "┛", "━", "┃"
	}
	inner := max(0, w-2)
	side := st.Render(vertical)
	out := make([]string, 0, h)
	out = append(out, st.Render(tl+strings.Repeat(horizontal, inner)+tr))
	for i := 0; i < h-2; i++ {
		l := ""
		if i < len(lines) {
			l = lines[i]
		}
		out = append(out, side+fitToWidth(l, inner)+side)
	}
	return append(out, st.Render(bl+strings.Repeat(horizontal, inner)+br))
}

// renderTreePanel draws the Sessions pane: its header (with the inbox count)
// over the foldable group → session → job tree.
func (m model) renderTreePanel(w, h int) []string {
	inner := w - 2
	lines := append([]string{m.treeHeaderCell(inner)}, m.treePaneLines(treeBodyHeight(h), inner)...)
	return paneBox(lines, w, h, m.jobs.tree.focus)
}

// renderDetailPanel draws the Details pane: the job (or, with the sidebar
// driving, the session) under the cursor, inset a column from the border.
func (m model) renderDetailPanel(w, h int) []string {
	lines := m.jobsDetailLines(max(0, h-2), max(0, w-4))
	for i, l := range lines {
		lines[i] = " " + l
	}
	return paneBox(lines, w, h, false)
}

// renderListPanel draws the Jobs pane: the view tabs, the column header and the
// job rows — or, while a modal is open, that modal in place of the table.
func (m model) renderListPanel(w, h int, overlay bool) []string {
	inner := w - 2
	strip, _, _ := m.renderMainTabStrip(inner)
	lines := []string{strip}
	// Column 0 of the list is a cursor gutter, like the sidebar's.
	cols := computeJobColumns(max(1, inner-1), m.jobsAnyTimeout())
	switch {
	case overlay:
		lines = append(lines, m.mgmtBodyLines(max(1, h-3), inner, cols)...)
	case m.jobs.main == mainAgents:
		lines = append(lines, m.agentViewHeader(inner))
		lines = append(lines, m.renderAgentView(inner, max(1, h-4))...)
	default:
		lines = append(lines, jobsHeaderStyle.Render(fitToWidth(" "+m.jobsHeaderRow(cols), inner)))
		lines = append(lines, m.mgmtBodyLines(max(1, h-4), inner, cols)...)
	}
	focused := !m.jobs.tree.focus || !m.jobsLayout().showTree
	out := paneBox(lines, w, h, focused)
	// The session strip scopes the *job table*, so it only earns the pane's top
	// row while the table is what is showing. Leaving it up in the Agents view
	// put a control that does nothing on the most prominent line of the screen.
	if !overlay && m.jobs.main == mainJobs {
		sessions, _ := m.jobsSessionStrip(inner)
		if focused {
			out[0] = focusBorderStyle.Render("┏") + sessions + focusBorderStyle.Render("┓")
		} else {
			out[0] = borderStyle.Render("╭") + sessions + borderStyle.Render("╮")
		}
	}
	return out
}

// mgmtBodyLines renders the visible window of the group→session→job list (or the
// edit-form overlay when active), returning exactly bodyH lines of width inner.
func (m model) mgmtBodyLines(bodyH, inner int, cols columnWidths) []string {
	if m.jobs.form.active {
		return m.sessionFormLines(bodyH, inner)
	}
	if m.jobs.settings.active {
		return m.settingsFormLines(bodyH, inner)
	}
	if m.jobs.picker.active {
		return m.sessionPickerLines(bodyH, inner)
	}
	if m.jobs.helping {
		return m.helpLines(bodyH, inner)
	}
	if m.jobs.tree.menu.active {
		return m.treeMenuLines(bodyH, inner)
	}
	out := make([]string, 0, bodyH)
	if len(m.jobs.rows) == 0 {
		// An empty queue is the normal state of a fresh daemon, so the empty
		// space says what to do next rather than just reporting emptiness.
		headline, hint := m.jobs.tab.emptyMessage()
		if m.jobs.filter != "" {
			headline, hint = "nothing matches /"+m.jobs.filter, "esc clears the filter"
		}
		for i := 0; i < bodyH; i++ {
			switch i {
			case bodyH / 2:
				out = append(out, treeSummaryStyle.Render(centerText(headline, inner)))
			case bodyH/2 + 1:
				out = append(out, treeEmptyStyle.Render(centerText(hint, inner)))
			default:
				out = append(out, "")
			}
		}
		return out
	}
	selLo, selHi := -1, -1
	if m.jobs.visual {
		selLo, selHi = m.visualRange()
	}
	for i := 0; i < bodyH; i++ {
		idx := m.jobs.offset + i
		if idx >= len(m.jobs.rows) {
			out = append(out, "")
			continue
		}
		r := m.jobs.rows[idx]
		isCursor := idx == m.jobs.cursor
		inSel := (m.jobs.visual && idx >= selLo && idx <= selHi) || m.jobs.tagged[r.job.ID]
		out = append(out, m.renderMgmtRow(r, cols, inner, isCursor, inSel))
	}
	return out
}

// renderMgmtRow renders one job row with the right highlight, keeping each
// column's semantic color under the focus tint / selection background.
//
// Column 0 is a gutter carrying the cursor bar. The row tint alone is too close
// to the panel to find at a glance — and it disappears entirely on a terminal
// that ignores background colors — so the bar is the cursor's real signal and
// the tint is the reinforcement. A tagged row gets its own marker there, which
// also keeps "tagged" distinguishable from "under the cursor" when it is both.
func (m model) renderMgmtRow(r mgmtRow, cols columnWidths, inner int, isCursor, inSel bool) string {
	body := inner - 1
	if body < 1 {
		body = 1
	}
	switch {
	case isCursor:
		bar := treeCursorBarStyle.Render("▌")
		return bar + padRowBg(renderJobsRow(r.job, r.group, cols, cRowFocusBg), body, cRowFocusBg)
	case inSel:
		return selectedStyle.Render("▌") +
			selectedStyle.Render(fitToWidth(stripAnsi(formatJobRow(r.job, r.group, cols)), body))
	default:
		return " " + formatJobRow(r.job, r.group, cols)
	}
}

// modalInnerWidth picks a modal's inner width: a comfortable fixed size that
// shrinks to fit narrow terminals.
func modalInnerWidth(bodyInner, prefer int) int {
	w := prefer
	if max := bodyInner - 4; w > max {
		w = max
	}
	if w < 24 {
		w = 24
	}
	return w
}

// modalBox wraps content lines in a rounded box with an accent title embedded in
// the top edge (╭─ Title ─╮), padding each line to innerW. Styled after bada's
// modalFrame/panelTop.
func modalBox(title string, content []string, innerW int) []string {
	titleR := modalTitleStyle.Render(title)
	dashes := innerW - 3 - lipgloss.Width(title)
	if dashes < 0 {
		dashes = 0
	}
	top := borderStyle.Render("╭─ ") + titleR + borderStyle.Render(" "+strings.Repeat("─", dashes)+"╮")
	side := borderStyle.Render("│")
	lines := []string{top}
	for _, c := range content {
		if w := lipgloss.Width(c); w < innerW {
			c += strings.Repeat(" ", innerW-w)
		} else if w > innerW {
			c = truncateToWidth(c, innerW)
		}
		lines = append(lines, side+c+side)
	}
	lines = append(lines, borderStyle.Render("╰"+strings.Repeat("─", innerW)+"╯"))
	return lines
}

// centerBox places a pre-rendered box centered within a bodyH×inner region.
func centerBox(box []string, bodyH, inner int) []string {
	boxW := 0
	for _, l := range box {
		if w := lipgloss.Width(l); w > boxW {
			boxW = w
		}
	}
	pad := strings.Repeat(" ", max(0, (inner-boxW)/2))
	top := max(0, (bodyH-len(box))/2)
	out := make([]string, 0, bodyH)
	for i := 0; i < bodyH; i++ {
		if ci := i - top; ci >= 0 && ci < len(box) {
			out = append(out, pad+box[ci])
		} else {
			out = append(out, "")
		}
	}
	return out
}

// helpLines renders the key-binding / command help overlay.
func (m model) helpLines(bodyH, inner int) []string {
	boxInner := modalInnerWidth(inner, 52)
	row := func(keys, desc string) string {
		return "  " + modalActiveStyle.Render(fmt.Sprintf("%-15s", keys)) + treeSummaryStyle.Render(desc)
	}
	content := []string{
		"",
		// What applies *here* comes first. The full list is long enough that at
		// 80x24 most of it is below the fold, so the top of the page has to be
		// the part you most likely came looking for.
		jobsDetailKeyStyle.Render("  Right now"),
	}
	for _, h := range m.jobsHints() {
		if _, _, ok := hintPress(h.key); ok || strings.Contains(h.key, "/") {
			content = append(content, row(h.key, h.label))
		}
	}
	content = append(content,
		"",
		jobsDetailKeyStyle.Render("  Navigation"),
		row("j / k", "move down / up"),
		row("gg / G", "jump to top / bottom"),
		row("^d / ^u", "half-page down / up"),
		"",
		jobsDetailKeyStyle.Render("  Sessions & jobs"),
		row("⏎", "job output (session if empty)"),
		row("o", "open the output pager"),
		row("S", "session picker: open/new/edit, d deletes"),
		row("  d on a ⊕", "keeps a worktree holding uncommitted work"),
		row("tab / ⇧tab", "roll the focused pane's views"),
		row("  list / tree", "jobs/agents · spaces/agents"),
		row("← / →", "roll status filters (from the job list)"),
		row(",n", "toggle session tree"),
		row("^w h/l", "focus tree (left) / list (right)"),
		row("  in tree", "l/o/␣ fold · h collapse · ⏎ open · e/n/d"),
		row("  m / A", "action menu · zoom the tree full-width"),
		row("e", "edit row: name/session/group"),
		row("n / a", "new session"),
		row("x / u / r", "kill / urgent / rerun job"),
		row("d", "remove job(s)"),
		"",
		jobsDetailKeyStyle.Render("  View & config"),
		row("sg/si/ss/st", "sort by field · repeat reverses"),
		row("R", "reverse the current sort"),
		row("/", "filter"),
		row("space", "select job + move down"),
		row("V", "visual multi-select"),
		row(":", "command line"),
		row("  tab", "complete · again cycles · ⇧tab back"),
		row(":set slots N", "set parallel jobs"),
		row(":set logdir P", "set log directory"),
		row(":config", "settings box (slots, logdir)"),
		row(":kill id|-a", "kill job(s) / all running"),
		row(":restart id|-a", "kill + re-enqueue job(s)"),
		row(":reset", "factory-reset runic (confirms)"),
		row("q", "quit"),
		row("esc", "back to the open session"),
		"",
		jobsDetailKeyStyle.Render("  In a session"),
		row("^B n/p", "next / previous session tab"),
		row("^B 1-9", "jump to the nth session tab"),
		row("^B o", "cycle panes within the session"),
		row("^B z", "zoom the focused pane (again restores)"),
		row("^B d", "detach back here (panes keep running)"),
		row("^B ?", "this help"),
		row("!", "acknowledge an agent without attaching"),
		"",
		jobsDetailKeyStyle.Render("  Reading the screen"),
		row("STATE", "green runs, amber waits, red failed"),
		row("nR nQ nF", "sidebar chip: running / queued / failed"),
		row("● ◐ ×", "an agent waits / works / broke, with how long"),
		row("◆ violet", "an agent row; the hue says it is calm"),
	)
	return m.helpViewport(content, bodyH, boxInner, inner)
}

// renderHWBar is the hardware section on the right of the status bar: CPU and
// memory with gauges, then load average and core count. Pieces that don't fit
// in w columns are dropped from the end.
func (m model) renderHWBar(w int) string {
	s := m.hwStats
	if !s.CPUOK && !s.MemOK && !s.LoadOK {
		return statusValueStyle.Foreground(cFgDim).Render(" gathering… (or unavailable on this platform) ")
	}
	var parts []string
	if s.CPUOK {
		val := statusValueStyle
		if s.CPUPercent >= 90 {
			val = val.Foreground(cRed).Bold(true)
		}
		parts = append(parts, statusPair("CPU", val.Render(fmt.Sprintf(" %3.0f%% %s ", s.CPUPercent, gauge(s.CPUPercent, 8)))))
	}
	if s.MemOK {
		val := statusValueStyle
		text := humanBytes(s.MemTotal)
		if s.MemUsed > 0 && s.MemTotal > 0 {
			// Same shape as the CPU segment: value + gauge, so the two read as
			// one system.
			pct := float64(s.MemUsed) / float64(s.MemTotal) * 100
			if pct >= 90 {
				val = val.Foreground(cRed).Bold(true)
			}
			text = fmt.Sprintf("%s/%s %s", humanBytes(s.MemUsed), humanBytes(s.MemTotal), gauge(pct, 8))
		}
		parts = append(parts, statusPair("MEM", val.Render(" "+text+" ")))
	}
	var quiet []string
	if s.LoadOK {
		quiet = append(quiet, fmt.Sprintf("load %.2f %.2f %.2f", s.Load[0], s.Load[1], s.Load[2]))
	}
	if s.NumCPU > 0 {
		quiet = append(quiet, fmt.Sprintf("%d cpu", s.NumCPU))
	}
	if len(quiet) > 0 {
		parts = append(parts, statusValueStyle.Foreground(cFgDim).Render(" "+strings.Join(quiet, " · ")+" "))
	}
	out := ""
	for _, p := range parts {
		if lipgloss.Width(out)+lipgloss.Width(p) > w {
			break
		}
		out += p
	}
	return out
}

// gauge renders a fixed-width █/░ bar for a 0..100 percentage.
func gauge(pct float64, width int) string {
	if width < 1 {
		return ""
	}
	filled := int(pct/100*float64(width) + 0.5)
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

// humanBytes formats a byte count with a compact binary suffix (e.g. 28.5G).
func humanBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	suffix := []string{"K", "M", "G", "T", "P"}
	if exp >= len(suffix) {
		exp = len(suffix) - 1
	}
	return fmt.Sprintf("%.1f%s", float64(b)/float64(div), suffix[exp])
}

func jobStateCounts(jobs []protocol.JobInfo) (running, queued, finished, failed int) {
	for _, j := range jobs {
		switch j.State {
		case protocol.StateRunning:
			running++
		case protocol.StateQueued, protocol.StateAllocating:
			queued++
		case protocol.StateFinished:
			if j.Result.Canceled {
				finished++
			} else if j.Result.DiedBySignal || j.Result.ExitCode != 0 {
				failed++
			} else {
				finished++
			}
		default:
			finished++
		}
	}
	return
}

func (m model) jobsDetailLines(maxLines, inner int) []string {
	out := make([]string, 0, maxLines)
	// A section heading is a word, not a hairline: the title takes readable
	// text color and only the trailing dashes stay at rule strength.
	rule := func(title string) {
		if lipgloss.Width(title) > inner {
			title = truncateToWidth(title, inner)
		}
		dashes := strings.Repeat("─", max(0, inner-lipgloss.Width(title)))
		out = append(out, jobsDetailTitleStyle.Render(title)+jobsDetailRuleStyle.Render(dashes))
	}
	// The panel follows the focus: driving the sidebar, it describes the session
	// under the cursor (branch, isolation, what its agent last said), which is
	// the context you need before opening or deleting it. Driving the list, it
	// describes the job.
	if lines, title, ok := m.sessionDetailLines(inner); ok {
		rule(title)
		for _, sl := range lines {
			if len(out) >= maxLines {
				break
			}
			out = append(out, sl)
		}
		for len(out) < maxLines {
			out = append(out, "")
		}
		return out[:maxLines]
	}
	if j, ok := m.jobsSelected(); ok {
		rule(" job details ")
		info := strings.Split(strings.TrimRight(format.FormatJobInfo(&j), "\n"), "\n")
		for _, il := range info {
			if len(out) >= maxLines {
				break
			}
			out = append(out, styleJobDetailLine(il, inner))
		}
	}
	for len(out) < maxLines {
		out = append(out, "")
	}
	return out[:maxLines]
}

// sessionDetailLines describes the group, session, or agent the sidebar cursor
// is on, together with the heading the panel should carry. The last result is
// false when the sidebar isn't driving, so the caller falls back to the job
// panel.
func (m model) sessionDetailLines(inner int) ([]string, string, bool) {
	if !m.jobs.tree.show || !m.jobs.tree.focus {
		return nil, "", false
	}
	r, ok := m.jobs.tree.current()
	if !ok || r.kind == treeJob {
		return nil, "", false
	}
	field := func(key, value string) string {
		k := jobsDetailKeyStyle.Render(key + ":")
		pad := strings.Repeat(" ", max(0, 11-lipgloss.Width(key)-1))
		return fitToWidth(k+pad+truncateToWidth(value, max(0, inner-12)), inner)
	}
	if r.kind == treeGroup {
		return []string{
			field("Group", r.group),
			field("Sessions", fmt.Sprintf("%d", r.total)),
			field("Jobs", jobsSummaryText(r)),
		}, " space ", true
	}
	if r.kind == treeAgent {
		// Ordered by what you need in order to decide what to do: what it is
		// doing, what it asked, what it printed, and only then the provenance.
		// The old panel led with the session and pane — true, but you already
		// know which row you are on — and truncated the question to one line,
		// which is the one thing on the panel you cannot act without.
		state := agentStateWord(r.attention, r.seen)
		if !r.since.IsZero() && r.attention != protocol.AttentionNone {
			state += " · " + waitAge(r.since)
		}
		if agentStalled(r) {
			state += " · quiet for " + durationCompact(agentQuietFor(r).Round(time.Minute))
		}
		lines := []string{field("State", state)}

		// The request, wrapped. A clipped question reads as a different, shorter
		// question.
		if r.note != "" {
			label := "Asked"
			if r.attention == protocol.AttentionIdle {
				label = "Result"
			}
			lines = append(lines, wrapField(inner, label, r.note)...)
		}
		// What it printed, for when "done" is all it said. Fetched per selection
		// rather than on every poll; see agentTailMsg.
		if snippet := m.agentSnippet(r); len(snippet) > 0 {
			lines = append(lines, field("Output", ""))
			for _, l := range snippet {
				// Tabs measure as one column and render as several, so they have
				// to go before the width is budgeted.
				l = strings.ReplaceAll(l, "\t", "    ")
				lines = append(lines, fitToWidth("  "+treeSummaryStyle.Render(truncateToWidth(l, max(8, inner-4))), inner))
			}
		}

		// Next action, as keys you can actually press from here.
		next := "⏎ attach"
		if protocol.Unresolved(r.attention, r.seen) {
			next += " · ! acknowledge"
		}
		lines = append(lines, field("Do", next))

		// Provenance last.
		if s, ok := m.sessionInfo(r.session); ok && s.Agent != "" {
			lines = append(lines, field("Agent", s.Agent+" in "+r.session+" · "+r.pane))
		} else {
			lines = append(lines, field("Agent", r.session+" · "+r.pane))
		}
		lines = append(lines, field("Group", r.group))
		if s, ok := m.sessionInfo(r.session); ok && s.Branch != "" {
			lines = append(lines, field("Branch", s.Branch))
		}
		if !r.lastOutput.IsZero() {
			lines = append(lines, field("Last output", waitAge(r.lastOutput)+" ago"))
		}
		return lines, " agent ", true
	}
	lines := []string{
		field("Session", r.session),
		field("Group", r.group),
	}
	// What the session hosts, as decided when it was created — a different fact
	// from what its panes are currently reporting, which is State below.
	if s, ok := m.sessionInfo(r.session); ok && s.Agent != "" {
		lines = append(lines, field("Agent", s.Agent))
	}
	if r.branch != "" {
		lines = append(lines,
			field("Branch", r.branch),
			field("Worktree", r.worktree))
	} else {
		lines = append(lines, field("Isolation", "none — panes open in runic's working directory"))
	}
	if r.attention != protocol.AttentionNone {
		state := r.attention.String()
		if !r.since.IsZero() {
			state += " for " + waitAge(r.since)
		}
		// "State", not "Agent": the row above may already name the kind of agent
		// the session hosts, and two lines labelled Agent would read as one
		// contradicting the other.
		lines = append(lines, field("State", state))
		if r.note != "" {
			lines = append(lines, field("Said", r.note))
		}
	}
	lines = append(lines, field("Jobs", jobsSummaryText(r)))
	return lines, " session ", true
}

// jobsSummaryText renders a tree row's job counts as one phrase.
func jobsSummaryText(r treeRow) string {
	if r.total == 0 && r.running == 0 && r.queued == 0 && r.failed == 0 {
		return "none"
	}
	var parts []string
	if r.running > 0 {
		parts = append(parts, fmt.Sprintf("%d running", r.running))
	}
	if r.queued > 0 {
		parts = append(parts, fmt.Sprintf("%d queued", r.queued))
	}
	if r.failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", r.failed))
	}
	if r.kind == treeSession {
		parts = append(parts, fmt.Sprintf("%d total", r.total))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, " · ")
}

func styleJobDetailLine(line string, inner int) string {
	if k, v, ok := strings.Cut(line, ":"); ok {
		key := jobsDetailKeyStyle.Render(k + ":")
		value := strings.TrimLeft(v, " ")
		return fitToWidth(key+" "+truncateToWidth(value, max(0, inner-lipgloss.Width(stripAnsi(key))-1)), inner)
	}
	return truncateToWidth(line, inner)
}

func (m model) confirmQuestion() string {
	switch m.jobs.confirm.kind {
	case confirmRemove:
		if n := len(m.jobs.confirm.ids); n != 1 {
			return fmt.Sprintf("Remove %d jobs?", n)
		}
		return fmt.Sprintf("Remove job %d?", m.jobs.confirm.ids[0])
	case confirmDeleteSession:
		return fmt.Sprintf("Delete session %q?", m.jobs.confirm.session)
	case confirmDiscardSession:
		// Name the loss, not just the action: this prompt is the last thing
		// between an agent's uncommitted work and `rm -rf`.
		return fmt.Sprintf("%q has %s — discard it?",
			m.jobs.confirm.session, shorten(m.jobs.confirm.detail, 70))
	case confirmDeleteGroup:
		return fmt.Sprintf("Delete group %q?", m.jobs.confirm.group)
	case confirmReset:
		return "Reset runic? Kills all jobs & panes, deletes sessions, restores default settings."
	}
	return "Clear all finished jobs?"
}

// jobsFooterLine is the row above the status bar, as in gorae: the open prompt
// (filter or command line) if any, else the confirmation question, else what
// may follow a pending two-key chord, else the context key hints.
func (m model) jobsFooterLine(w int) string {
	line, _ := m.jobsFooterSpans(w)
	return line
}

// jobsFooterSpans renders the footer and reports where its hints landed, so the
// same function answers both the renderer and a click.
func (m model) jobsFooterSpans(w int) (string, []hintSpan) {
	// Offset the spans by whatever leads the bar: a confirm question and a chord
	// prefix both push the hints to the right.
	withLead := func(lead string, hints []keyHint) (string, []hintSpan) {
		leadW := lipgloss.Width(lead)
		bar, spans := renderHintBar(hints, max(0, w-leadW))
		for i := range spans {
			spans[i].start += leadW
			spans[i].end += leadW
		}
		return fitToWidth(lead+bar, w), spans
	}
	switch {
	case m.jobs.filtering:
		return fitToWidth(inputStyle.Render("/")+m.textInput.View(), w), nil
	case m.jobs.commanding:
		return fitToWidth(inputStyle.Render(m.cmdInput.View()), w), nil
	case m.jobs.confirm.kind != confirmNone:
		lead := dangerChipStyle.Render(" CONFIRM ") + " " + finishedErrStyle.Render(m.confirmQuestion()) + "  "
		return withLead(lead, []keyHint{{key: "y", label: "yes"}, {key: "n/esc", label: "cancel"}})
	}
	if prefix, hints := m.pendingHints(); prefix != "" {
		return withLead(warnChipStyle.Render(" "+prefix+"… ")+" ", hints)
	}
	return renderHintBar(m.jobsHints(), w)
}

// keyHint is one entry of the hint bar: a key cap followed by a short label.
//
// keep marks a hint that must survive a bar too narrow for all of them. The bar
// drops from the end, so without it the hints that matter most when you are
// lost — help, quit, cancel — are the first to go, being the ones conventionally
// listed last.
type keyHint struct {
	key, label string
	keep       bool
}

// hintSpan is where one hint landed on the rendered bar, so a click can be
// resolved back to the key it advertises. See clickHintBar.
type hintSpan struct {
	hint       keyHint
	start, end int // [start, end) columns from the bar's left edge
}

// renderHintBar draws key caps ( j  move   tab  views …) on one row, dropping
// trailing hints that don't fit rather than wrapping. The spans it returns make
// the bar clickable: it already lists exactly the keys that apply right now, so
// turning those into targets gives a complete mouse path with no new UI.
func renderHintBar(hints []keyHint, width int) (string, []hintSpan) {
	hintWidth := func(h keyHint) int {
		return lipgloss.Width(keyCapStyle.Render(" "+h.key+" ") + " " + keyLabelStyle.Render(h.label))
	}
	// Space the kept hints will need, held back from everything optional so an
	// early hint cannot spend the room the last one depends on.
	reserved := 0
	for _, h := range hints {
		if h.keep {
			reserved += hintWidth(h) + 2
		}
	}

	var b strings.Builder
	var spans []hintSpan
	used := 0
	for i, h := range hints {
		chip := keyCapStyle.Render(" "+h.key+" ") + " " + keyLabelStyle.Render(h.label)
		cw := lipgloss.Width(chip)
		sep := 0
		if i > 0 {
			sep = 2
		}
		if h.keep {
			reserved -= cw + 2
		} else if used+sep+cw+reserved > width {
			// No room for this one *and* the kept hints still to come.
			continue
		}
		if used+sep+cw > width {
			continue
		}
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(chip)
		// The label is part of the target, not just the cap: a two-column chip
		// is a miserable thing to hit, and the whole entry reads as one button.
		spans = append(spans, hintSpan{hint: h, start: used + sep, end: used + sep + cw})
		used += sep + cw
	}
	return fitToWidth(b.String(), width), spans
}

// pendingHints lists what may follow a pending chord prefix, so sequences like
// s… and ,… are discoverable instead of flashing by.
func (m model) pendingHints() (string, []keyHint) {
	switch m.jobs.pending {
	case 's':
		return "sort", []keyHint{{key: "g", label: "group"}, {key: "i", label: "id"}, {key: "s", label: "state"}, {key: "t", label: "time"}, {key: "any", label: "cancel"}}
	case 'g':
		return "g", []keyHint{{key: "g", label: "top"}, {key: "any", label: "cancel"}}
	case ',':
		return ",", []keyHint{{key: "n", label: "toggle session tree"}}
	}
	return "", nil
}

// jobsHints are the context hints for the management screen, ordered by
// usefulness: the ones that don't fit are dropped from the end.
func (m model) jobsHints() []keyHint {
	if m.jobs.helping {
		return []keyHint{{key: "any", label: "close help"}}
	}
	if m.jobs.form.active || m.jobs.settings.active {
		return []keyHint{{key: "tab", label: "next field"}, {key: "enter", label: "save"}, {key: "esc", label: "cancel"}}
	}
	if m.jobs.picker.active || m.jobs.tree.menu.active {
		return []keyHint{{key: "j/k", label: "move"}, {key: "enter", label: "choose"}, {key: "esc", label: "close"}}
	}
	if m.jobs.main == mainAgents && !(m.jobs.tree.show && m.jobs.tree.focus) {
		return m.agentViewHints()
	}
	if m.jobs.tree.show && m.jobs.tree.focus {
		// The Agents view is flat, so the fold and group-management keys are
		// left out of its hints rather than advertised and then ignored.
		if m.jobs.tree.tab == sideAgents {
			return []keyHint{
				{key: "j/k", label: "move"}, {key: "tab", label: "spaces"}, {key: "⏎", label: "attach"},
				{key: "m", label: "menu"}, {key: "e", label: "edit"}, {key: "^w l", label: "jobs"},
				{key: "A", label: "zoom"},
				{key: "?", label: "help", keep: true}, {key: "q", label: "quit", keep: true},
			}
		}
		return []keyHint{
			{key: "j/k", label: "move"}, {key: "tab", label: "agents"}, {key: "l/h", label: "fold"}, {key: "⏎", label: "open"},
			{key: "m", label: "menu"}, {key: "e", label: "edit"}, {key: "n", label: "new"}, {key: "d", label: "delete"},
			{key: "^w l", label: "jobs"}, {key: "A", label: "zoom"},
			{key: "?", label: "help", keep: true}, {key: "q", label: "quit", keep: true},
		}
	}
	if m.jobs.visual {
		return []keyHint{{key: "j/k", label: "extend"}, {key: "x", label: "kill"}, {key: "r", label: "rerun"}, {key: "u", label: "urgent"}, {key: "d", label: "remove"}, {key: "esc", label: "cancel"}}
	}
	// `^w h` is listed early and named for its payoff: the sidebar's own tabs
	// are only reachable once it has focus, and a user looking at the Agents
	// tab from here has no other way to learn that.
	hints := []keyHint{
		{key: "j/k", label: "move"}, {key: "[/]", label: "session"}, {key: "tab", label: "agents"}, {key: "←/→", label: "status"}, {key: "^w h", label: "sidebar"}, {key: "⏎", label: "output"},
		{key: "/", label: "filter"}, {key: ":", label: "command"}, {key: "space", label: "select"}, {key: "s", label: "sort"},
		{key: "x", label: "kill"}, {key: "r", label: "rerun"}, {key: "d", label: "remove"}, {key: "S", label: "sessions"},
		{key: ",n", label: "tree"}, {key: "?", label: "help"}, {key: "q", label: "quit"},
	}
	if len(m.jobs.tagged) > 0 {
		hints = append([]keyHint{{key: "esc", label: "clear selection"}}, hints...)
	}
	return hints
}

// jobsStatusBar is the bottom row: the mode chip, what narrows the table (view,
// filter, selection), the last message, and the hardware readout on the right.
func (m model) jobsStatusBar(w int) string {
	sessions, needYou := 0, 0
	for _, n := range m.nodes {
		sessions += len(n.sessions)
		for _, s := range n.sessions {
			// An acknowledged error stays visible in the tree but stops
			// counting here: you already know about it.
			if protocol.Unresolved(s.Attention, s.AttentionSeen) {
				needYou++
			}
		}
	}
	mode, modeStyle := " MANAGE ", chipStyle
	switch {
	case m.jobs.confirm.kind != confirmNone:
		mode, modeStyle = " CONFIRM ", dangerChipStyle
	case m.jobs.commanding:
		mode, modeStyle = " COMMAND ", modeCommandStyle
	case m.jobs.filtering:
		mode, modeStyle = " FILTER ", modeCommandStyle
	case m.jobs.visual:
		lo, hi := m.visualRange()
		mode, modeStyle = fmt.Sprintf(" VISUAL %d ", hi-lo+1), modeInsertStyle
	}
	val := func(s string) string { return statusValueStyle.Render(" " + s + " ") }

	left := modeStyle.Render(mode) + statusPair("VIEW", val(m.jobs.tab.label()))
	if needYou > 0 {
		// The one count you are meant to act on gets the loudest chip. It names
		// its unit because the sidebar's Agents tab carries a count of its own,
		// over *panes* — a session with two blocked agents is "1 session" here
		// and "2!" there, and an unlabelled number would read as a contradiction.
		noun := "sessions need you"
		if needYou == 1 {
			noun = "session needs you"
		}
		left += " " + dangerChipStyle.Render(fmt.Sprintf(" %d %s ", needYou, noun))
	}
	// The count says how many; the alarm says which one and why. It is the
	// half you can act on without going looking, so it sits beside the count.
	if !m.alarm.alert.empty() {
		left += " " + dangerChipStyle.Render(" ! "+shorten(m.alarm.alert.text(), 48)+" ")
	}
	left += statusPair("SESS", val(fmt.Sprint(sessions)))
	if n := len(m.jobs.tagged); n > 0 {
		left += statusPair("SEL", val(fmt.Sprint(n)))
	}
	if m.jobs.filter != "" {
		left += statusPair("FILTER", val("/"+shorten(m.jobs.filter, 24)))
	}
	switch {
	case m.status == "":
		left += statusPair("MSG", val("Ready"))
	case strings.HasPrefix(m.status, "error"):
		left += " " + dangerChipStyle.Render(" ERR ") + val(shorten(m.status, 60))
	default:
		left += statusPair("MSG", val(shorten(m.status, 60)))
	}

	right := ""
	if room := w - lipgloss.Width(left) - 1; room > 0 {
		right = m.renderHWBar(room)
	}
	return renderStatusBar(w, left, right)
}

// joinExact pads/truncates to exactly h lines of width w and joins them.
func joinExact(lines []string, w, h int) string {
	for len(lines) < h {
		lines = append(lines, fitToWidth("", w))
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	return strings.Join(lines, "\n")
}

func centerText(s string, w int) string {
	n := lipgloss.Width(s)
	if n >= w {
		return s
	}
	return strings.Repeat(" ", (w-n)/2) + s
}

func shorten(s string, n int) string {
	if lipgloss.Width(s) <= n || n <= 1 {
		return s
	}
	return truncateToWidth(s, n-1) + "…"
}

func (m model) jobsBordered(content string, inner int) string {
	b := borderStyle.Render("│")
	return b + fitToWidth(content, inner) + b
}

// helpViewport frames the help content as a scrollable page. The list is far
// longer than a terminal row budget — at 80x24 well over half of it is below
// the fold — and it used to be handed to centerBox, which silently emitted only
// the first bodyH lines. Everything past "View & config" was unreachable, with
// nothing on screen saying so.
func (m model) helpViewport(content []string, bodyH, boxInner, inner int) []string {
	// Two border rows, a title row, and the footer line this adds.
	view := max(1, bodyH-4)
	offset := m.jobs.help.offset
	if maxOff := max(0, len(content)-view); offset > maxOff {
		offset = maxOff
	}
	page := content[offset:min(len(content), offset+view)]

	// Say where you are, so a clipped page is visibly clipped.
	pos := "all"
	switch {
	case len(content) <= view:
	case offset == 0:
		pos = "top"
	case offset+view >= len(content):
		pos = "end"
	default:
		pos = fmt.Sprintf("%d%%", offset*100/max(1, len(content)-view))
	}
	foot := fmt.Sprintf("  j/k ^d/^u scroll · esc close   %s", pos)
	if len(content) <= view {
		foot = "  esc close"
	}
	page = append(append([]string{}, page...), "", helpStyle.Render(fitToWidth(foot, boxInner-2)))
	return centerBox(modalBox("Help", page, boxInner), bodyH, inner)
}

// helpScroll moves the help page, clamped. The content length is not known here
// (it is built during render), so the floor is 0 and the ceiling is applied by
// helpViewport against the real list.
func (m *model) helpScroll(delta int) {
	m.jobs.help.offset = max(0, m.jobs.help.offset+delta)
}

// wrapField renders a labelled value over as many lines as it needs, with the
// continuation lines aligned under the value. A truncated question reads as a
// different, shorter question, which is the one thing on an agent's panel you
// cannot act without.
func wrapField(inner int, label, value string) []string {
	const labelW = 12
	valueW := max(16, inner-labelW-2)
	words := strings.Fields(value)
	if len(words) == 0 {
		return nil
	}
	var out []string
	line := ""
	flush := func(head string) {
		pad := strings.Repeat(" ", labelW)
		if head != "" {
			pad = fmt.Sprintf("%-*s", labelW, head+":")
		}
		// Padded to the panel width, like the single-line fields: a short line
		// leaves the panel's background ragged against the border.
		out = append(out, fitToWidth(jobsDetailKeyStyle.Render(pad)+treeSummaryStyle.Render(line), inner))
	}
	head := label
	for _, w := range words {
		candidate := w
		if line != "" {
			candidate = line + " " + w
		}
		if lipgloss.Width(candidate) > valueW && line != "" {
			flush(head)
			head, line = "", w
			continue
		}
		line = candidate
	}
	if line != "" {
		flush(head)
	}
	return out
}
