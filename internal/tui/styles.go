package tui

import "github.com/charmbracelet/lipgloss"

// --- theme -----------------------------------------------------------------
//
// Herdr-inspired terminal chrome: charcoal surfaces, blue focus, compact tabs,
// and quiet labels. Light mode uses warm paper and the same semantic hierarchy.
// All colors flow through these tokens so the jobs, agents, shells, and dialogs
// stay consistent. Status colors remain distinct from the focus accent.
//
// Text colors are deliberately brighter than Herdr's faintest labels so they
// remain readable in terminal fonts. TestThemeContrast checks text against both
// panel and selection surfaces in light and dark modes.
var (
	// Surfaces: progressively lighter panels on dark, darker on light.
	cBarBg    = lipgloss.AdaptiveColor{Dark: "#15181D", Light: "#E5E3DD"} // deepest bar
	cPanelBg  = lipgloss.AdaptiveColor{Dark: "#1A1D22", Light: "#F0EEE9"} // pane fill
	cHeaderBg = lipgloss.AdaptiveColor{Dark: "#20242C", Light: "#E5E3DD"} // table header / info

	// Text: soft gray on dark, warm ink on light. The ramp is short and
	// high-contrast — every step still clears AA on the panel.
	cFgBright = lipgloss.AdaptiveColor{Dark: "#F0F2F5", Light: "#1A1A18"}
	cFg       = lipgloss.AdaptiveColor{Dark: "#C8CDD4", Light: "#3A3A36"}
	cFgMuted  = lipgloss.AdaptiveColor{Dark: "#B2B8C2", Light: "#4B4B46"}
	cFgFaint  = lipgloss.AdaptiveColor{Dark: "#A1A8B3", Light: "#55554F"}
	cFgDim    = lipgloss.AdaptiveColor{Dark: "#979FAA", Light: "#595953"}

	// Hairlines / borders: raised well above the panel fill so frames and
	// dividers are actually visible.
	cRule   = lipgloss.AdaptiveColor{Dark: "#2B3038", Light: "#D8D6D0"}
	cBorder = lipgloss.AdaptiveColor{Dark: "#414854", Light: "#A6A49C"}

	// Accents: hues carry semantic meaning across the whole TUI, at full
	// saturation on dark and at full pigment depth on light.
	cAccent = lipgloss.AdaptiveColor{Dark: "#69ADFF", Light: "#174EA6"} // blue — focus/normal
	cGreen  = lipgloss.AdaptiveColor{Dark: "#8ECF7A", Light: "#28651C"} // running / insert
	cAmber  = lipgloss.AdaptiveColor{Dark: "#E6C060", Light: "#765000"} // queued / waiting
	cRed    = lipgloss.AdaptiveColor{Dark: "#F08B8B", Light: "#A12532"} // error / failed
	cInfo   = lipgloss.AdaptiveColor{Dark: "#A6BDE0", Light: "#345581"} // detail keys, labels

	// Badges use contrasting ink; pale selections have their own dark text.
	cInk      = lipgloss.AdaptiveColor{Dark: "#1A1D22", Light: "#FFFFFF"}
	cCursorBg = cAccent
	cSelectBg = lipgloss.AdaptiveColor{Dark: "#A6BDE0", Light: "#BDCEE5"}
	cSelectFg = lipgloss.AdaptiveColor{Dark: "#192230", Light: "#1A1A18"}
	cInactBg  = lipgloss.AdaptiveColor{Dark: "#20242C", Light: "#E9E7E1"}
	cInactFg  = cFgMuted

	// cRowFocusBg tints the focused row in the jobs table. Unlike the solid
	// accent bar, it is dark/desaturated enough that each cell keeps its own
	// semantic foreground (running=green, failed=red, …) and stays legible.
	cRowFocusBg = lipgloss.AdaptiveColor{Dark: "#282E38", Light: "#DCDDD9"}

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
	// treeSummaryStyle is also what a calm agent row is drawn in. The tree's
	// hierarchy is carried by weight — group bold, session bright, agent faint,
	// job in its state color — rather than by giving "is an agent" a hue of its
	// own. Spending color on a row's *category* leaves nothing louder for its
	// urgency, which is the only thing worth interrupting you for.
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

	// Compact chrome: blue active tabs, quiet shortcut labels, and subdued
	// status labels keep attention on workspace content.
	chipStyle        = lipgloss.NewStyle().Bold(true).Foreground(cInk).Background(cAccent)
	warnChipStyle    = lipgloss.NewStyle().Bold(true).Foreground(cInk).Background(cAmber)
	dangerChipStyle  = lipgloss.NewStyle().Bold(true).Foreground(cInk).Background(cRed)
	tabInactiveStyle = lipgloss.NewStyle().Foreground(cFg)
	keyCapStyle      = lipgloss.NewStyle().Foreground(cFgMuted).Background(cBarBg)
	keyLabelStyle    = lipgloss.NewStyle().Foreground(cFgDim)
	statusLabelStyle = lipgloss.NewStyle().Foreground(cFgDim).Background(cBarBg)
	statusValueStyle = lipgloss.NewStyle().Foreground(cFg).Background(cBarBg)
	paneTitleStyle   = lipgloss.NewStyle().Bold(true).Foreground(cFgBright)
)
