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
// unhighlighted rows (bg == nil) it is a no-op — jobsBordered pads those.
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
	if w < 8 || h < 4 {
		return strings.Repeat("\n", max(0, h-1))
	}
	inner := w - 2
	bodyH := m.jobsBodyHeight()
	lines := make([]string, 0, h)

	// Overlays (help / edit form / session picker) take the whole box; the tree
	// sidebar only shares space with the plain list, and only if there's room.
	overlay := m.jobs.helping || m.jobs.form.active || m.jobs.settings.active ||
		m.jobs.picker.active || m.jobs.tree.menu.active
	showTree := m.jobs.tree.show && !overlay && inner >= 46
	// NerdTree-style zoom (`A`): the tree fills the box; the list is hidden. The
	// divider still costs a column so the tree cell width is inner-1.
	zoom := showTree && m.jobs.tree.zoom

	treeW := treePaneWidth
	listInner := inner
	if showTree {
		if zoom {
			treeW = inner - 1
			listInner = 0
		} else {
			listInner = inner - treePaneWidth - 1
		}
	}
	// Column 0 of the list is a cursor gutter, like the sidebar's.
	cols := computeJobColumns(max(1, listInner-1))

	// A row is [tree cell][divider][list line] when the sidebar is shown.
	treeLines := []string(nil)
	treeIdx := 0
	if showTree {
		treeLines = m.treePaneLines(bodyH+jobsDetailHeight, treeW)
	}
	withTree := func(right string) string {
		if !showTree {
			return right
		}
		cell := strings.Repeat(" ", treeW)
		if treeIdx < len(treeLines) {
			cell = treeLines[treeIdx]
		}
		treeIdx++
		if zoom {
			return cell + treeDividerCell()
		}
		return cell + treeDividerCell() + right
	}

	lines = append(lines, boxedTop(w, m.jobsBoxTitle(), borderStyle))
	header := jobsHeaderStyle.Render(fitToWidth(" "+m.jobsHeaderRow(cols), listInner))
	if zoom {
		header = m.treeHeaderCell(treeW) + treeDividerCell()
	} else if showTree {
		header = m.treeHeaderCell(treeW) + treeDividerCell() + header
	}
	lines = append(lines, m.jobsBordered(header, inner))

	regionH := bodyH
	if overlay {
		regionH = bodyH + jobsDetailHeight
	}
	body := m.mgmtBodyLines(regionH, listInner, cols)
	for _, bl := range body {
		lines = append(lines, m.jobsBordered(withTree(bl), inner))
	}

	if !overlay {
		for _, dl := range m.jobsDetailLines(jobsDetailHeight, listInner) {
			lines = append(lines, m.jobsBordered(withTree(dl), inner))
		}
	}

	lines = append(lines, boxedBottom(w, borderStyle))
	if m.jobs.filtering {
		lines = append(lines, fitToWidth(inputStyle.Render("/")+m.textInput.View(), w))
	}
	if m.jobs.commanding {
		lines = append(lines, fitToWidth(inputStyle.Render(m.cmdInput.View()), w))
		// Candidates from an ambiguous tab, so completion is a visible choice
		// rather than a blind rotation.
		lines = append(lines, m.completionLines(w)...)
	}
	lines = append(lines, m.jobsFooter(w))
	lines = append(lines, m.renderHWBar(w))

	return joinExact(lines, w, h)
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
		headline, hint := "no jobs yet", "submit one with  ru <command>   ·  ? for help"
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
		row(",n / tab", "toggle session tree · focus tree ⇄ list"),
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

// renderHWBar is the system-wide hardware status bar shown at the bottom of the
// job view: CPU, memory, load average, and core count.
func (m model) renderHWBar(w int) string {
	s := m.hwStats
	left := []statusSegment{{text: " HW ", style: airlineMuted}}
	if !s.CPUOK && !s.MemOK && !s.LoadOK {
		left = append(left, statusSegment{text: " gathering… (or unavailable on this platform) ", style: airlineMuted})
		return renderAirline(w, left, nil)
	}
	if s.CPUOK {
		style := airlineInfo
		if s.CPUPercent >= 90 {
			style = airlineError
		}
		left = append(left, statusSegment{
			text:  fmt.Sprintf(" CPU %3.0f%% %s ", s.CPUPercent, gauge(s.CPUPercent, 8)),
			style: style,
		})
	}
	if s.MemOK {
		style := airlineInfo
		if s.MemTotal > 0 && float64(s.MemUsed)/float64(s.MemTotal) >= 0.9 {
			style = airlineError
		}
		memText := fmt.Sprintf(" MEM %s ", humanBytes(s.MemTotal))
		if s.MemUsed > 0 && s.MemTotal > 0 {
			// Same shape as the CPU segment: value + gauge, so the two chips
			// read as one system.
			pct := float64(s.MemUsed) / float64(s.MemTotal) * 100
			memText = fmt.Sprintf(" MEM %s/%s %s ", humanBytes(s.MemUsed), humanBytes(s.MemTotal), gauge(pct, 8))
		}
		left = append(left, statusSegment{text: memText, style: style})
	}

	// One quiet segment on the right instead of two competing chips.
	var parts []string
	if s.LoadOK {
		parts = append(parts, fmt.Sprintf("load %.2f %.2f %.2f", s.Load[0], s.Load[1], s.Load[2]))
	}
	if s.NumCPU > 0 {
		parts = append(parts, fmt.Sprintf("%d cpu", s.NumCPU))
	}
	var right []statusSegment
	if len(parts) > 0 {
		right = append(right, statusSegment{text: " " + strings.Join(parts, " · ") + " ", style: airlineMuted})
	}
	return renderAirline(w, left, right)
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

func (m model) jobsBordered(content string, inner int) string {
	b := borderStyle.Render("│")
	return b + fitToWidth(content, inner) + b
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
	rule := func(title string) {
		r := title + strings.Repeat("─", max(0, inner-lipgloss.Width(title)))
		out = append(out, jobsDetailRuleStyle.Render(fitToWidth(r, inner)))
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

func (m model) jobsFooter(w int) string {
	if m.jobs.confirm.kind != confirmNone {
		var q string
		switch m.jobs.confirm.kind {
		case confirmRemove:
			if n := len(m.jobs.confirm.ids); n == 1 {
				q = fmt.Sprintf(" remove job %d? (y/n) ", m.jobs.confirm.ids[0])
			} else {
				q = fmt.Sprintf(" remove %d jobs? (y/n) ", n)
			}
		case confirmDeleteSession:
			q = fmt.Sprintf(" delete session %q? (y/n) ", m.jobs.confirm.session)
		case confirmDiscardSession:
			// Name the loss, not just the action: this prompt is the last thing
			// between an agent's uncommitted work and `rm -rf`.
			q = fmt.Sprintf(" %q has %s — discard it? (y/n) ",
				m.jobs.confirm.session, shorten(m.jobs.confirm.detail, 70))
		case confirmDeleteGroup:
			q = fmt.Sprintf(" delete group %q? (y/n) ", m.jobs.confirm.group)
		case confirmReset:
			q = " reset runic? kills all jobs & panes, deletes sessions, restores default settings (y/n) "
		default:
			q = " clear all finished jobs? (y/n) "
		}
		return renderAirline(w, []statusSegment{
			{text: " CONFIRM ", style: airlineError},
			{text: q, style: airlineInfo},
		}, nil)
	}
	running, queued, finished, failed := jobStateCounts(m.visibleJobs())
	sessions, needYou := 0, 0
	for _, n := range m.nodes {
		sessions += len(n.sessions)
		for _, s := range n.sessions {
			if s.Attention.NeedsYou() {
				needYou++
			}
		}
	}
	modeText, modeStyle := " MANAGE ", modeCommandStyle
	if m.jobs.visual {
		lo, hi := m.visualRange()
		modeText, modeStyle = fmt.Sprintf(" VISUAL %d ", hi-lo+1), modeInsertStyle
	}
	// The sort field/direction already shows as an arrow on the sorted column
	// header, so it isn't repeated here — the left side stays just the mode
	// plus transient context (selection, filter, last action).
	left := []statusSegment{{text: modeText, style: modeStyle}}
	if n := len(m.jobs.tagged); n > 0 {
		left = append(left, statusSegment{text: fmt.Sprintf(" sel %d ", n), style: modeInsertStyle})
	}
	if m.jobs.filter != "" {
		left = append(left, statusSegment{text: fmt.Sprintf(" /%s ", m.jobs.filter), style: airlineInfo})
	}
	if m.status != "" {
		left = append(left, statusSegment{text: " " + shorten(m.status, 60) + " ", style: airlineInfo})
	}
	// Right side stays calm: activity counts appear only when non-zero, so an
	// idle queue shows just the session count and the help pointer. Failures get
	// their own red chip.
	var right []statusSegment
	// Sessions whose panes reported "waiting on you" or "errored" get the loudest
	// chip on the bar: it is the one count you are meant to act on.
	if needYou > 0 {
		right = append(right, statusSegment{text: fmt.Sprintf(" %d need you ", needYou), style: airlineError})
	}
	if failed > 0 {
		right = append(right, statusSegment{text: fmt.Sprintf(" fail %d ", failed), style: airlineError})
	}
	var counts []string
	if running > 0 {
		counts = append(counts, fmt.Sprintf("run %d", running))
	}
	if queued > 0 {
		counts = append(counts, fmt.Sprintf("queue %d", queued))
	}
	if finished > 0 {
		counts = append(counts, fmt.Sprintf("done %d", finished))
	}
	counts = append(counts, fmt.Sprintf("sess %d", sessions))
	right = append(right,
		statusSegment{text: " " + strings.Join(counts, " · ") + " ", style: airlineMuted},
		statusSegment{text: " ? help ", style: airlineFocus},
	)
	return renderAirline(w, left, right)
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

// jobsBoxTitle names the management box in its top border: the app, plus
// whatever narrows what is on screen (an active filter, a tag selection), so
// the frame answers "why am I not seeing everything?" without a second glance.
func (m model) jobsBoxTitle() string {
	title := " runic "
	switch {
	case m.jobs.filter != "":
		title = fmt.Sprintf(" runic · /%s ", shorten(m.jobs.filter, 24))
	case len(m.jobs.tagged) > 0:
		title = fmt.Sprintf(" runic · %d selected ", len(m.jobs.tagged))
	}
	return title
}
