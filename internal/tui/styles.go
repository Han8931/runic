package tui

import "github.com/charmbracelet/lipgloss"

// --- theme -----------------------------------------------------------------
//
// "Deep": cool navy surfaces under a bright cyan accent, after gorae's default
// look. All colors flow through the semantic tokens below, each a
// lipgloss.AdaptiveColor — near-black navy panels with saturated accents in dark
// mode, cool paper with deep-pigment accents in light mode. To retheme the whole
// app, edit these tokens, not the styles beneath them.
//
// Cyan is the accent: focus, the active tab, key caps, the mode chip. The state
// hues keep their meaning across the whole TUI — green runs, amber waits, red
// broke — so none of them doubles as decoration. Every token here is pinned by
// TestThemeContrast, which holds the whole palette to WCAG AA against the
// surfaces it is actually drawn on — retune freely, but run the test.
var (
	// Surfaces: progressively lighter panels on dark, darker on light.
	cBarBg    = lipgloss.AdaptiveColor{Dark: "#070A0F", Light: "#DCE0E8"} // deepest bar
	cPanelBg  = lipgloss.AdaptiveColor{Dark: "#0A0F14", Light: "#EFF1F5"} // pane fill
	cHeaderBg = lipgloss.AdaptiveColor{Dark: "#141D27", Light: "#E2E6EE"} // table header / info

	// Text: cool white on dark, ink on light. The ramp is short and
	// high-contrast — every step still clears AA on the panel.
	cFgBright = lipgloss.AdaptiveColor{Dark: "#F8FAFC", Light: "#1E2030"}
	cFg       = lipgloss.AdaptiveColor{Dark: "#EEF1F6", Light: "#3B3E56"}
	cFgMuted  = lipgloss.AdaptiveColor{Dark: "#B6C0CE", Light: "#4C4F69"}
	cFgFaint  = lipgloss.AdaptiveColor{Dark: "#A0ABBB", Light: "#55586F"}
	cFgDim    = lipgloss.AdaptiveColor{Dark: "#909DAF", Light: "#5C5F77"}

	// Hairlines / borders: raised well above the panel fill so frames and
	// dividers are actually visible.
	cRule   = lipgloss.AdaptiveColor{Dark: "#263443", Light: "#CCD0DA"}
	cBorder = lipgloss.AdaptiveColor{Dark: "#3A4B5E", Light: "#9CA0B0"}

	// Accents: hues carry semantic meaning across the whole TUI, at full
	// saturation on dark and at full pigment depth on light.
	cAccent = lipgloss.AdaptiveColor{Dark: "#5CC8FF", Light: "#1747B0"} // cyan — focus/normal
	cGreen  = lipgloss.AdaptiveColor{Dark: "#A3E635", Light: "#1F5F18"} // running / insert
	cAmber  = lipgloss.AdaptiveColor{Dark: "#FBBF24", Light: "#7A4A00"} // queued / waiting
	cRed    = lipgloss.AdaptiveColor{Dark: "#FB7185", Light: "#A30D2E"} // error / failed
	cInfo   = lipgloss.AdaptiveColor{Dark: "#5EEAD4", Light: "#0B5C56"} // detail keys, labels

	// Badges use contrasting ink; pale selections have their own dark text.
	cInk      = lipgloss.AdaptiveColor{Dark: "#0A0F14", Light: "#FFFFFF"}
	cCursorBg = cAccent
	cSelectBg = lipgloss.AdaptiveColor{Dark: "#5EEAD4", Light: "#A6DBD2"}
	cSelectFg = lipgloss.AdaptiveColor{Dark: "#062A26", Light: "#1E2030"}
	cInactBg  = lipgloss.AdaptiveColor{Dark: "#121A23", Light: "#E4E8F0"}
	cInactFg  = cFgMuted

	// cRowFocusBg tints the focused row in the jobs table. Unlike the solid
	// accent bar, it is dark/desaturated enough that each cell keeps its own
	// semantic foreground (running=green, failed=red, …) and stays legible.
	cRowFocusBg = lipgloss.AdaptiveColor{Dark: "#1A2C3E", Light: "#D2DCF2"}

	// Neutral headers leave the accent for focus and active controls.
	cHeaderTint = cHeaderBg
)

var (
	groupStyle       = lipgloss.NewStyle().Bold(true).Foreground(cFg)
	sessionStyle     = lipgloss.NewStyle().Foreground(cFgBright)
	treeIconStyle    = lipgloss.NewStyle().Foreground(cAccent)
	folderStyle      = lipgloss.NewStyle().Foreground(cAmber)
	modalTitleStyle  = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	modalActiveStyle = lipgloss.NewStyle().Bold(true).Foreground(cFgBright)
	treeSummaryStyle = lipgloss.NewStyle().Foreground(cFgFaint)
	treeEmptyStyle   = lipgloss.NewStyle().Italic(true).Foreground(cFgDim)
	treePaneStyle    = lipgloss.NewStyle().Background(cPanelBg)
	cursorStyle      = lipgloss.NewStyle().Foreground(cInk).Background(cCursorBg)
	// inputCursorStyle is the text-input caret: a maximum-contrast solid block
	// (white in dark mode, near-black in light). NOTE: bubbles' cursor.View
	// applies Reverse(true) on top of this style, swapping fg/bg at render
	// time — so the *foreground* here is the block color the user sees.
	inputCursorStyle    = lipgloss.NewStyle().Foreground(cFgBright).Background(cInk)
	cursorInactiveStyle = lipgloss.NewStyle().Foreground(cInactFg).Background(cInactBg)
	// treeCursorDimStyle marks the sidebar's cursor row while the list (not the
	// tree) has focus, so its position stays visible without competing.
	treeCursorDimStyle = lipgloss.NewStyle().Foreground(cInactFg).Background(cInactBg)
	// The sidebar's cursor bar: a solid glyph in column 0. It is the one cursor
	// signal that does not depend on background colors rendering as intended,
	// so it carries the accent at full strength when the tree has focus and
	// fades — but stays — when it does not.
	treeCursorBarStyle    = lipgloss.NewStyle().Bold(true).Foreground(cAccent).Background(cRowFocusBg)
	treeCursorDimBarStyle = lipgloss.NewStyle().Foreground(cBorder).Background(cInactBg)
	// Attention styles color a session row by what its panes have reported.
	// Only the two "needs you" states get a loud hue; a pane merely working is
	// informational and must not compete with a genuine failure.
	attnWaitingStyle = lipgloss.NewStyle().Bold(true).Foreground(cAmber)
	attnErrorStyle   = lipgloss.NewStyle().Bold(true).Foreground(cRed)
	// Working is healthy activity, so it borrows the running hue rather than a
	// warning one: on this palette, warm means "look at me".
	attnWorkingStyle    = lipgloss.NewStyle().Foreground(cGreen)
	selectedStyle       = lipgloss.NewStyle().Foreground(cSelectFg).Background(cSelectBg)
	cursorSelectedStyle = lipgloss.NewStyle().Foreground(cSelectFg).Background(cSelectBg)
	runningStyle        = lipgloss.NewStyle().Bold(true).Foreground(cGreen)
	queuedStyle         = lipgloss.NewStyle().Foreground(cAmber)
	finishedStyle       = lipgloss.NewStyle().Foreground(cFgFaint)
	finishedErrStyle    = lipgloss.NewStyle().Bold(true).Foreground(cRed)
	skippedStyle        = lipgloss.NewStyle().Foreground(cFgMuted)
	jobIDStyle          = lipgloss.NewStyle().Foreground(cFgMuted)
	jobNameStyle        = lipgloss.NewStyle().Foreground(cFgBright)
	statusBarStyle      = lipgloss.NewStyle().Background(cBarBg).Foreground(cFgDim)
	inputStyle          = lipgloss.NewStyle().Foreground(cInfo)
	helpStyle           = lipgloss.NewStyle().Foreground(cFgFaint)
	borderStyle         = lipgloss.NewStyle().Foreground(cBorder)
	focusBorderStyle    = lipgloss.NewStyle().Foreground(cAccent)
	modeInsertStyle     = lipgloss.NewStyle().Bold(true).Foreground(cInk).Background(cGreen) // green
	modeCommandStyle    = lipgloss.NewStyle().Bold(true).Foreground(cInk).Background(cAmber) // orange
	// jobsHeaderStyle block-highlights the table header row: bright bold text on
	// the gentle header tint.
	jobsHeaderStyle     = lipgloss.NewStyle().Bold(true).Foreground(cFgBright).Background(cHeaderTint)
	jobsDetailKeyStyle  = lipgloss.NewStyle().Foreground(cInfo)
	jobsDetailRuleStyle = lipgloss.NewStyle().Foreground(cRule)
	// jobsDetailTitleStyle is a detail-panel section heading; cRule is far too
	// faint to carry words.
	jobsDetailTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(cFgMuted)

	// gorae-style chrome: filled chips for the mode and the active tab, key caps
	// in the hint bar, and LABEL value pairs on the status bar.
	chipStyle        = lipgloss.NewStyle().Bold(true).Foreground(cInk).Background(cAccent)
	warnChipStyle    = lipgloss.NewStyle().Bold(true).Foreground(cInk).Background(cAmber)
	dangerChipStyle  = lipgloss.NewStyle().Bold(true).Foreground(cInk).Background(cRed)
	tabInactiveStyle = lipgloss.NewStyle().Foreground(cFg)
	keyCapStyle      = lipgloss.NewStyle().Bold(true).Foreground(cAccent).Background(cHeaderBg)
	keyLabelStyle    = lipgloss.NewStyle().Foreground(cFgDim)
	statusLabelStyle = lipgloss.NewStyle().Bold(true).Foreground(cInfo).Background(cBarBg)
	statusValueStyle = lipgloss.NewStyle().Foreground(cFg).Background(cBarBg)
	paneTitleStyle   = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
)
