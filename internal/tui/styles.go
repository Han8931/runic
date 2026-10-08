package tui

import "github.com/charmbracelet/lipgloss"

// --- theme -----------------------------------------------------------------
//
// "Ember": deep oxblood surfaces under a vivid yellow-to-red accent ramp. All
// colors flow through the semantic tokens below, each a lipgloss.AdaptiveColor
// — warm dark-maroon panels with saturated accents in dark mode, cream paper
// with deep-pigment accents in light mode. To retheme the whole app, edit these
// tokens, not the styles beneath them.
//
// The surfaces are a color, not an absence of one: a dark maroon reads as a lit
// room rather than a void, and it gives the three panel layers a visible order
// at the low contrast steps a TUI has room for. What keeps that from going
// muddy is holding the accents at full saturation and well clear of the fill in
// brightness — half-saturated accents on a mid-tone fill sit at similar
// luminance, and a terminal with no antialiasing renders that as a smear.
//
// The accents are a ramp — gold for attention, orange for waiting, red for
// broken — because the states they mark are a ramp too, and reading "hotter
// means worse" needs no legend. Green stays for running: it is the one state
// that must never be mistaken for a problem, so it is the one hue outside the
// ramp. Every token here is pinned by TestThemeContrast, which holds the whole
// palette to WCAG AA against the surfaces it is actually drawn on — retune
// freely, but run the test.
var (
	// Surfaces: progressively lighter panels on dark, darker on light.
	cBarBg    = lipgloss.AdaptiveColor{Dark: "#25100F", Light: "#FFFAF0"} // deepest bar
	cPanelBg  = lipgloss.AdaptiveColor{Dark: "#331814", Light: "#FFF4E2"} // pane fill
	cHeaderBg = lipgloss.AdaptiveColor{Dark: "#45211B", Light: "#FFE3BE"} // table header / info
	cSubtleBg = lipgloss.AdaptiveColor{Dark: "#5A2C22", Light: "#FFCF91"} // focused airline segment

	// Text: warm cream on dark, near-black on light. The ramp is short and
	// high-contrast — every step still clears AA on the panel.
	cFgBright = lipgloss.AdaptiveColor{Dark: "#FFF6EC", Light: "#2A0F08"}
	cFg       = lipgloss.AdaptiveColor{Dark: "#FBE9DC", Light: "#3A1A10"}
	cFgMuted  = lipgloss.AdaptiveColor{Dark: "#EFD2C0", Light: "#5E3222"}
	cFgFaint  = lipgloss.AdaptiveColor{Dark: "#E7C7B3", Light: "#663828"}
	cFgDim    = lipgloss.AdaptiveColor{Dark: "#DFBDA8", Light: "#6E3F2D"}

	// Hairlines / borders: raised well above the panel fill so frames and
	// dividers are actually visible.
	cRule   = lipgloss.AdaptiveColor{Dark: "#6B342A", Light: "#E6C49F"}
	cBorder = lipgloss.AdaptiveColor{Dark: "#9A5241", Light: "#B57A52"}

	// Accents: hues carry semantic meaning across the whole TUI, at full
	// saturation on dark and at full pigment depth on light.
	cAccent = lipgloss.AdaptiveColor{Dark: "#FFD447", Light: "#7A4300"} // gold — focus/normal
	cGreen  = lipgloss.AdaptiveColor{Dark: "#A8EA62", Light: "#2F5A0E"} // running / insert
	cAmber  = lipgloss.AdaptiveColor{Dark: "#FFA64D", Light: "#94380A"} // queued / waiting
	cRed    = lipgloss.AdaptiveColor{Dark: "#FF8A8A", Light: "#AD0017"} // error / failed
	cInfo   = lipgloss.AdaptiveColor{Dark: "#FFCFA0", Light: "#6B3410"} // detail keys, prompts

	// Badges use contrasting ink; pale selections have their own dark text.
	cInk      = lipgloss.AdaptiveColor{Dark: "#25100F", Light: "#FFFFFF"}
	cCursorBg = cAccent
	cSelectBg = lipgloss.AdaptiveColor{Dark: "#FFD9A0", Light: "#FFCF91"}
	cSelectFg = lipgloss.AdaptiveColor{Dark: "#2A1209", Light: "#2A1A10"}
	cInactBg  = lipgloss.AdaptiveColor{Dark: "#3F201B", Light: "#FFEBD3"}
	cInactFg  = cFgMuted

	// cRowFocusBg tints the focused row in the jobs table. Unlike the solid
	// accent bar, it is dark/desaturated enough that each cell keeps its own
	// semantic foreground (running=green, failed=red, …) and stays legible.
	cRowFocusBg = lipgloss.AdaptiveColor{Dark: "#4C251E", Light: "#FFE0B8"}

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
	airlineMode         = lipgloss.NewStyle().Bold(true).Foreground(cInk).Background(cAccent)
	modeNormalStyle     = lipgloss.NewStyle().Bold(true).Foreground(cInk).Background(cAccent) // gold
	modeInsertStyle     = lipgloss.NewStyle().Bold(true).Foreground(cInk).Background(cGreen)  // green
	modeCommandStyle    = lipgloss.NewStyle().Bold(true).Foreground(cInk).Background(cAmber)  // orange
	branchStyle         = lipgloss.NewStyle().Foreground(cFgMuted).Background(cHeaderBg)
	airlineFocus        = lipgloss.NewStyle().Foreground(cFgBright).Background(cSubtleBg)
	airlineInfo         = lipgloss.NewStyle().Foreground(cFg).Background(cHeaderBg)
	airlineMuted        = lipgloss.NewStyle().Foreground(cFgMuted).Background(cBarBg)
	airlineError        = lipgloss.NewStyle().Bold(true).Foreground(cInk).Background(cRed)
	headerActive        = lipgloss.NewStyle().Bold(true).Foreground(cInk).Background(cAccent)
	headerInactive      = lipgloss.NewStyle().Bold(true).Foreground(cFgDim).Background(cBarBg)
	// jobsHeaderStyle block-highlights the table header row: bright bold text on
	// the gentle header tint.
	jobsHeaderStyle     = lipgloss.NewStyle().Bold(true).Foreground(cFgBright).Background(cHeaderTint)
	jobsDetailKeyStyle  = lipgloss.NewStyle().Foreground(cInfo)
	jobsDetailRuleStyle = lipgloss.NewStyle().Foreground(cRule)
)
