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

type columnWidths struct {
	id, group, session, state, tm, timeout, name, command int
}

func computeJobColumns(inner int) columnWidths {
	// state fits the widest badge ("× signal 15", "◆ retry 1/2") without
	// clipping the glyph the column is read by.
	c := columnWidths{id: 5, group: 10, session: 12, state: 12, tm: 8, timeout: 8, name: 14}
	const sep = 9 // three spaces after ID, single space between the other columns
	fixedNoCmd := func() int {
		return c.id + c.group + c.session + c.state + c.tm + c.timeout + c.name + sep
	}
	c.command = inner - fixedNoCmd()
	if c.command < 10 {
		// Reclaim room from the widest text columns first.
		for _, p := range []*int{&c.name, &c.session, &c.group} {
			if c.command >= 10 {
				break
			}
			need := 10 - c.command
			room := *p - 6
			if room > need {
				room = need
			}
			if room > 0 {
				*p -= room
				c.command = inner - fixedNoCmd()
			}
		}
	}
	if c.command < 1 {
		c.command = 1
	}
	return c
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
	// The label lives in its own NAME column; unnamed jobs echo a dim command
	// snippet there so the column still identifies the row at a glance.
	nameTxt, nameStyle := j.Label, jobNameStyle
	if nameTxt == "" {
		nameTxt, nameStyle = j.Command, treeEmptyStyle
	}
	stateTxt, stateStyle := jobStateText(j)
	cell := func(text string, w int, style lipgloss.Style) string {
		if bg != nil {
			style = style.Background(bg)
		}
		return style.Render(fitToWidth(stripAnsi(text), w))
	}
	sep := " "
	if bg != nil {
		sep = lipgloss.NewStyle().Background(bg).Render(" ")
	}
	id := cell(fmt.Sprintf("%*d", c.id, j.ID), c.id, jobIDStyle)
	grp := cell(group, c.group, groupStyle)
	sess := cell(displaySession(j.Session), c.session, sessionStyle)
	state := cell(stateTxt, c.state, stateStyle)
	tm := cell(timeStr, c.tm, lipgloss.NewStyle().Foreground(cFg))
	timeoutTxt, timeoutStyle := "-", treeEmptyStyle
	if j.TimeoutMS > 0 {
		timeoutTxt, timeoutStyle = durationCompact(time.Duration(j.TimeoutMS)*time.Millisecond), queuedStyle
	}
	timeout := cell(timeoutTxt, c.timeout, timeoutStyle)
	name := cell(nameTxt, c.name, nameStyle)
	command := cell(j.Command, c.command, lipgloss.NewStyle().Foreground(cFg))
	return id + sep + sep + sep + grp + sep + sess + sep + state + sep + tm + sep + timeout + sep + name + sep + command
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
	return fmt.Sprintf("%*s", c.id, mark("ID", m.jobs.sortMode == sortByID)) + "   " +
		fitToWidth(mark("GROUP", m.jobs.sortMode == sortGrouped), c.group) + " " +
		fitToWidth("SESSION", c.session) + " " +
		fitToWidth(mark("STATE", m.jobs.sortMode == sortByState), c.state) + " " +
		fitToWidth(mark("TIME", m.jobs.sortMode == sortByTime), c.tm) + " " +
		fitToWidth("TIMEOUT", c.timeout) + " " +
		fitToWidth("NAME", c.name) + " " +
		fitToWidth("COMMAND", c.command)
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
	if focused {
		st = focusBorderStyle
	}
	inner := max(0, w-2)
	side := st.Render("│")
	out := make([]string, 0, h)
	out = append(out, st.Render("╭"+strings.Repeat("─", inner)+"╮"))
	for i := 0; i < h-2; i++ {
		l := ""
		if i < len(lines) {
			l = lines[i]
		}
		out = append(out, side+fitToWidth(l, inner)+side)
	}
	return append(out, st.Render("╰"+strings.Repeat("─", inner)+"╯"))
}

// renderTreePanel draws the Sessions pane: its header (with the inbox count)
// over the foldable group → session → job tree.
func (m model) renderTreePanel(w, h int) []string {
	inner := w - 2
	lines := append([]string{m.treeHeaderCell(inner)}, m.treePaneLines(max(0, h-3), inner)...)
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
	strip, _ := m.renderTabStrip(inner)
	lines := []string{strip}
	// Column 0 of the list is a cursor gutter, like the sidebar's.
	cols := computeJobColumns(max(1, inner-1))
	if overlay {
		lines = append(lines, m.mgmtBodyLines(max(1, h-3), inner, cols)...)
	} else {
		lines = append(lines, jobsHeaderStyle.Render(fitToWidth(" "+m.jobsHeaderRow(cols), inner)))
		lines = append(lines, m.mgmtBodyLines(max(1, h-4), inner, cols)...)
	}
	return paneBox(lines, w, h, !m.jobs.tree.focus || !m.jobs.tree.show)
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
		row("tab / ⇧tab", "roll views: all/running/queued/done/failed"),
		row("← / →", "roll views (from the job list)"),
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
		jobsDetailKeyStyle.Render("  Reading the screen"),
		row("STATE", "green runs, amber waits, red failed"),
		row("nR nQ nF", "sidebar chip: running / queued / failed"),
		row("● ◐ ×", "an agent waits / works / broke, with how long"),
		"",
		helpStyle.Render("  press any key to close"),
	}
	return centerBox(modalBox("Help", content, boxInner), bodyH, inner)
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
	if lines, ok := m.sessionDetailLines(inner); ok {
		rule(" session ")
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

// sessionDetailLines describes the session (or group) the sidebar cursor is on.
// The second result is false when the sidebar isn't driving, so the caller
// falls back to the job panel.
func (m model) sessionDetailLines(inner int) ([]string, bool) {
	if !m.jobs.tree.show || !m.jobs.tree.focus {
		return nil, false
	}
	r, ok := m.jobs.tree.current()
	if !ok || r.kind == treeJob {
		return nil, false
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
		}, true
	}
	lines := []string{
		field("Session", r.session),
		field("Group", r.group),
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
		lines = append(lines, field("Agent", state))
		if r.note != "" {
			lines = append(lines, field("Said", r.note))
		}
	}
	lines = append(lines, field("Jobs", jobsSummaryText(r)))
	return lines, true
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
	switch {
	case m.jobs.filtering:
		return fitToWidth(inputStyle.Render("/")+m.textInput.View(), w)
	case m.jobs.commanding:
		return fitToWidth(inputStyle.Render(m.cmdInput.View()), w)
	case m.jobs.confirm.kind != confirmNone:
		lead := dangerChipStyle.Render(" CONFIRM ") + " " + finishedErrStyle.Render(m.confirmQuestion()) + "  "
		return fitToWidth(lead+renderHintBar([]keyHint{{"y", "yes"}, {"n/esc", "cancel"}}, max(0, w-lipgloss.Width(lead))), w)
	}
	if prefix, hints := m.pendingHints(); prefix != "" {
		lead := warnChipStyle.Render(" "+prefix+"… ") + " "
		return fitToWidth(lead+renderHintBar(hints, max(0, w-lipgloss.Width(lead))), w)
	}
	return renderHintBar(m.jobsHints(), w)
}

// keyHint is one entry of the hint bar: a key cap followed by a short label.
type keyHint struct{ key, label string }

// renderHintBar draws key caps ( j  move   tab  views …) on one row, dropping
// trailing hints that don't fit rather than wrapping.
func renderHintBar(hints []keyHint, width int) string {
	var b strings.Builder
	used := 0
	for i, h := range hints {
		chip := keyCapStyle.Render(" "+h.key+" ") + " " + keyLabelStyle.Render(h.label)
		cw := lipgloss.Width(chip)
		sep := 0
		if i > 0 {
			sep = 2
		}
		if used+sep+cw > width {
			break
		}
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(chip)
		used += sep + cw
	}
	return fitToWidth(b.String(), width)
}

// pendingHints lists what may follow a pending chord prefix, so sequences like
// s… and ,… are discoverable instead of flashing by.
func (m model) pendingHints() (string, []keyHint) {
	switch m.jobs.pending {
	case 's':
		return "sort", []keyHint{{"g", "group"}, {"i", "id"}, {"s", "state"}, {"t", "time"}, {"any", "cancel"}}
	case 'g':
		return "g", []keyHint{{"g", "top"}, {"any", "cancel"}}
	case ',':
		return ",", []keyHint{{"n", "toggle session tree"}}
	}
	return "", nil
}

// jobsHints are the context hints for the management screen, ordered by
// usefulness: the ones that don't fit are dropped from the end.
func (m model) jobsHints() []keyHint {
	if m.jobs.helping {
		return []keyHint{{"any", "close help"}}
	}
	if m.jobs.form.active || m.jobs.settings.active {
		return []keyHint{{"tab", "next field"}, {"enter", "save"}, {"esc", "cancel"}}
	}
	if m.jobs.picker.active || m.jobs.tree.menu.active {
		return []keyHint{{"j/k", "move"}, {"enter", "choose"}, {"esc", "close"}}
	}
	if m.jobs.tree.show && m.jobs.tree.focus {
		return []keyHint{
			{"j/k", "move"}, {"tab", "views"}, {"l/h", "fold"}, {"⏎", "open"},
			{"m", "menu"}, {"e", "edit"}, {"n", "new"}, {"d", "delete"},
			{"^w l", "jobs"}, {"A", "zoom"}, {"?", "help"}, {"q", "quit"},
		}
	}
	if m.jobs.visual {
		return []keyHint{{"j/k", "extend"}, {"x", "kill"}, {"r", "rerun"}, {"u", "urgent"}, {"d", "remove"}, {"esc", "cancel"}}
	}
	hints := []keyHint{
		{"j/k", "move"}, {"tab", "views"}, {"⏎", "output"}, {"/", "filter"},
		{":", "command"}, {"space", "select"}, {"s", "sort"}, {"x", "kill"},
		{"r", "rerun"}, {"d", "remove"}, {"S", "sessions"}, {",n", "tree"},
		{"?", "help"}, {"q", "quit"},
	}
	if len(m.jobs.tagged) > 0 {
		hints = append([]keyHint{{"esc", "clear selection"}}, hints...)
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
			if s.Attention.NeedsYou() {
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
		// The one count you are meant to act on gets the loudest chip.
		left += " " + dangerChipStyle.Render(fmt.Sprintf(" %d need you ", needYou))
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
