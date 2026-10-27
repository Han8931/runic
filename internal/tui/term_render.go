package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Rendering for the split view: the outer box around the terminal region, each
// tiled pane's cells, and the separators between them.

func (m model) View() string {
	if m.width == 0 {
		return "loading..."
	}
	// A pending bell leads the frame. It rides along with the frame's single
	// write instead of going to stdout separately, which is what keeps it from
	// landing inside one of the renderer's escape sequences. BEL occupies no
	// cell, so it does not disturb the layout.
	if m.viewMode == viewJobs {
		return m.bellPrefix() + m.renderJobsView(m.width, m.height)
	}

	var b strings.Builder
	headerHeight := 1
	contentHeight := m.height - 1 - headerHeight
	if m.inputMode != inputNone {
		contentHeight--
	}
	if m.inputMode == inputCommand {
		contentHeight -= m.completionHeight()
	}
	if contentHeight < 3 {
		contentHeight = 3
	}

	regionW, _ := m.termRegionSize()
	// The region spans the header line plus the content rows; line 0 is the
	// box top border, lines 1..contentHeight are the content/bottom rows.
	regionLines := m.renderTermRegion(regionW, contentHeight+1)
	sideW := m.termSidebarWidth()
	var sidebar []string
	if sideW > 0 {
		sidebar = m.renderSessionSidebar(sideW, contentHeight+1)
	}
	if m.jobs.form.active || m.jobs.tree.menu.active {
		regionLines = m.renderListPanel(regionW, contentHeight+1, true)
	}
	if !m.jobs.form.active && !m.jobs.tree.menu.active {
		strip, _ := m.sessionStrip(regionW, m.sessionNames(), m.activeSession)
		regionLines[0] = strip
	}

	for row := 0; row < contentHeight+1; row++ {
		if sideW > 0 {
			b.WriteString(sidebar[row])
			b.WriteString(" ")
		}
		if row < len(regionLines) {
			b.WriteString(regionLines[row])
		} else {
			b.WriteString(fitToWidth("", regionW))
		}
		b.WriteByte('\n')
	}

	if m.inputMode == inputCommand {
		b.WriteString(m.modeStyle().Render(" COMMAND "))
		b.WriteString(" ")
		b.WriteString(m.cmdInput.View())
		b.WriteByte('\n')
		for _, line := range m.completionLines(regionW) {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}

	if m.inputMode == inputNone {
		switch {
		case m.ctrlBPressed:
			// The prefix is live: say what it can do. A chord you have to
			// already know is not a feature anyone discovers.
			line, _ := m.prefixMenuLine(m.width)
			b.WriteString(line)
		case m.jobs.confirm.kind != confirmNone || m.jobs.form.active || m.jobs.tree.menu.active:
			b.WriteString(m.jobsFooterLine(m.width))
		default:
			b.WriteString(m.renderFooter())
		}
	}

	return m.bellPrefix() + b.String()
}

// renderTermRegion renders the whole right-hand terminal region — the rounded
// outer box, every tiled pane's contents, and the separators between panes —
// into exactly `height` styled lines, each `width` display columns wide.
//
// The box's top line is a placeholder: View always replaces it, with the
// session-tab strip or (with a form open) the list panel, so nothing titled is
// drawn here. The pane count the title used to carry is on the status bar.
func (m model) renderTermRegion(width, height int) []string {
	focused := m.focus == paneTerm && !m.jobs.tree.focus

	grid := make([][]string, height)
	for y := range grid {
		grid[y] = make([]string, width)
		for x := range grid[y] {
			grid[y][x] = " "
		}
	}

	if width >= 4 && height >= 3 {
		iw, ih := width-2, height-2
		leaves, seps := m.termLeaves(iw, ih)
		focusRect := rect{}
		haveFocus := false
		for _, lr := range leaves {
			if lr.node == m.focusPane {
				focusRect = lr.rect
				haveFocus = true
			}
		}

		for _, lr := range leaves {
			m.placePane(grid, lr, focused && lr.node == m.focusPane)
		}

		for _, sp := range seps {
			st := termBorderStyle(focused && haveFocus && sepTouchesRect(sp, focusRect))
			if sp.vertical {
				cx := 1 + sp.x
				for i := 0; i < sp.length; i++ {
					cy := 1 + sp.y + i
					if cy > 0 && cy < height-1 && cx > 0 && cx < width-1 {
						grid[cy][cx] = st.Render("│")
					}
				}
			} else {
				cy := 1 + sp.y
				for i := 0; i < sp.length; i++ {
					cx := 1 + sp.x + i
					if cy > 0 && cy < height-1 && cx > 0 && cx < width-1 {
						grid[cy][cx] = st.Render("─")
					}
				}
			}
		}
	}

	m.drawOuterBorder(grid, width, height, termBorderStyle(focused))

	lines := make([]string, height)
	for y := 0; y < height; y++ {
		var b strings.Builder
		for x := 0; x < width; x++ {
			b.WriteString(grid[y][x])
		}
		lines[y] = fitToWidth(b.String(), width)
	}
	return lines
}

// placePane draws one pane's terminal contents into the grid at its rectangle
// (offset by 1,1 to clear the outer border).
func (m model) placePane(grid [][]string, lr leafRect, showCursor bool) {
	r := lr.rect
	if r.w < 1 || r.h < 1 {
		return
	}
	sh := lr.node.shell
	done := sh == nil
	if sh != nil {
		sh.mu.Lock()
		done = sh.done
		sh.mu.Unlock()
	}

	var cells [][]string
	if done {
		cells = make([][]string, r.h)
		for y := range cells {
			cells[y] = make([]string, r.w)
			for x := range cells[y] {
				cells[y][x] = " "
			}
		}
		for i, ch := range "[shell exited]" {
			if i >= r.w {
				break
			}
			cells[0][i] = helpStyle.Render(string(ch))
		}
	} else {
		cells = renderTermCells(sh.vt, r.w, r.h, showCursor)
	}

	for yy := 0; yy < r.h && yy < len(cells); yy++ {
		for xx := 0; xx < r.w && xx < len(cells[yy]); xx++ {
			gy := 1 + r.y + yy
			gx := 1 + r.x + xx
			if gy >= 0 && gy < len(grid) && gx >= 0 && gx < len(grid[gy]) {
				grid[gy][gx] = cells[yy][xx]
			}
		}
	}
}

func (m model) drawOuterBorder(grid [][]string, width, height int, st lipgloss.Style) {
	if width < 2 || height < 2 {
		return
	}
	top := boxedTop(width, "", st)
	bottom := boxedBottom(width, st)
	for x, ch := range []rune(stripAnsi(top)) {
		if x < width {
			grid[0][x] = st.Render(string(ch))
		}
	}
	for x, ch := range []rune(stripAnsi(bottom)) {
		if x < width {
			grid[height-1][x] = st.Render(string(ch))
		}
	}
	for y := 1; y < height-1; y++ {
		grid[y][0] = st.Render("│")
		grid[y][width-1] = st.Render("│")
	}
}

func boxedTop(width int, title string, st lipgloss.Style) string {
	maxTitle := width - 4
	if maxTitle < 0 {
		maxTitle = 0
	}
	if lipgloss.Width(title) > maxTitle {
		title = strings.TrimRight(truncateToWidth(title, maxTitle), " ")
	}
	remaining := width - 2 - lipgloss.Width(title)
	if remaining < 0 {
		remaining = 0
	}
	return st.Render("╭" + title + strings.Repeat("─", remaining) + "╮")
}

func boxedBottom(width int, st lipgloss.Style) string {
	if width < 2 {
		return st.Render(strings.Repeat("─", width))
	}
	return st.Render("╰" + strings.Repeat("─", width-2) + "╯")
}

// sepTouchesRect reports whether a separator borders the given rectangle, used
// to highlight the separators around the focused pane.
func sepTouchesRect(sp separator, r rect) bool {
	if sp.vertical {
		if sp.x != r.x-1 && sp.x != r.x+r.w {
			return false
		}
		return sp.y < r.y+r.h && sp.y+sp.length > r.y
	}
	if sp.y != r.y-1 && sp.y != r.y+r.h {
		return false
	}
	return sp.x < r.x+r.w && sp.x+sp.length > r.x
}

func termBorderStyle(focused bool) lipgloss.Style {
	if focused {
		return focusBorderStyle
	}
	return borderStyle
}

func clearScreenCmd() tea.Cmd {
	return func() tea.Msg {
		return tea.ClearScreen()
	}
}
