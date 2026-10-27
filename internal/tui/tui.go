package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/han/runic/internal/client"
	"github.com/han/runic/internal/config"
	"github.com/han/runic/internal/protocol"
	"github.com/han/runic/internal/sysmon"
)

type pane int

const (
	paneTerm pane = iota
)

type model struct {
	width  int
	height int
	focus  pane

	ctrlWPressed bool
	ctrlWTimer   int
	ctrlBPressed bool
	ctrlBTimer   int
	nextPaneID   int

	nodes  []treeNode
	groups []string // all group names, from the last tree poll
	// agents is every live pane from the last poll, which the sidebar's Agents
	// tab lists. A session rolls its panes up to one state; this keeps the
	// individual panes, so two agents in one session stay two rows.
	agents []protocol.TerminalInfo
	// agentKinds is the configured agent registry's kinds, which the New-session
	// box offers. Read from the same config the daemon resolves them against.
	agentKinds []string
	// alarm tracks which panes have already rung, so a newly blocked agent
	// alarms once rather than on every poll. See alarm.go.
	alarm     alarmState
	maxSlots  int
	err       error
	status    string
	inputMode inputKind
	textInput textinput.Model
	cmdInput  textinput.Model
	// completion is the `:` command line's tab-completion state, shared by both
	// command modes (the pane view's and the management view's).
	completion completionState

	// layouts holds each session's split-layout tree (tmux-style tiling).
	// panes is a flat id→shell registry used for PTY event routing. shell and
	// focusPane track the currently focused pane within the active session.
	layouts       map[string]*paneNode
	panes         map[int]*shellState
	focusPane     *paneNode
	shell         *shellState
	activeSession string
	gitBranch     string

	// pendingOpen names a session just created via the management form that
	// should be opened as soon as it appears in the next tree refresh.
	pendingOpen string

	// viewMode selects the top-level screen: the normal split view or the
	// full-screen job-management modal. jobs holds that modal's state.
	viewMode viewMode
	jobs     jobsView

	// jobsOnly is set when the TUI was launched directly into the
	// job-management view (`ru --jobs`). In that mode, quitting the jobs
	// view exits the program instead of dropping into the split view.
	jobsOnly bool

	// hwMon samples system-wide hardware metrics shown in the job view's
	// status bar; hwStats holds the latest snapshot.
	hwMon   *sysmon.Monitor
	hwStats sysmon.Stats

	// mouseOn tracks whether terminal mouse reporting is currently enabled. It
	// supports session tabs and sidebar navigation in both views. Shift-drag
	// bypasses reporting for native selection in terminals that support it.
	mouseOn           bool
	termSidebarHidden bool
	// lastClick backs double-click detection on both screens; see clicks.go.
	lastClick clickState
	// spinnerFrame advances the working-agent animation; spinnerOn guards
	// against stacking timers. See spinner.go.
	spinnerFrame int
	spinnerOn    bool
	// snippet caches the selected agent's output tail; see snippet.go.
	snippet agentSnippetState
	// paneZoom gives the focused pane the whole region, tmux-style. A
	// full-screen agent or a wall of test output is unreadable in a quarter of
	// the screen, and splitting to read something then un-splitting to get the
	// layout back loses the layout.
	paneZoom bool

	// tuiWatch is the TUI-registration connection; takenOver is set when the
	// daemon displaced this TUI because a newer `ru` attached.
	tuiWatch  *client.Client
	takenOver bool
}

type inputKind int

const (
	inputNone inputKind = iota
	inputCreate
	inputRename
	inputMove
	inputCommand
)

type treeDataMsg struct {
	groups   []string
	sessions []protocol.SessionInfo
	jobs     []protocol.JobInfo
	// terminals is every live daemon-hosted pane, which the sidebar's Agents
	// tab lists one row per. Sessions carry only the rolled-up attention, so
	// the per-pane states have to come from their own query.
	terminals    []protocol.TerminalInfo
	maxSlots     int
	openJobsView bool
	err          error
}

type tickMsg time.Time
type commaTimeoutMsg struct{ id int }
type ctrlWTimeoutMsg struct{ id int }
type ctrlBTimeoutMsg struct{ id int }
type gTimeoutMsg struct{ id int }

type actionDoneMsg struct {
	status                 string
	err                    error
	renamedFrom, renamedTo string
	// offerDiscard names a session whose delete was refused because its worktree
	// holds uncommitted work. It escalates to the discard confirmation instead of
	// leaving the user with an error and no way forward; detail is what the
	// daemon said was in the way.
	offerDiscard string
	detail       string
}

// newTextInput returns a textinput whose cursor is always visible while the
// field is focused. The static mode matters: blink updates aren't routed to
// these inputs, so a blinking cursor would simply never show. The explicit
// block style matters too — the default cursor is bare reverse-video, which
// some terminal/theme combinations render invisibly.
func newTextInput() textinput.Model {
	ti := textinput.New()
	ti.Cursor.SetMode(cursor.CursorStatic)
	ti.Cursor.Style = inputCursorStyle
	return ti
}

func newModel(sh *shellState) model {
	ti := newTextInput()
	ti.CharLimit = 64

	ci := newTextInput()
	ci.CharLimit = 128
	ci.Prompt = ":"

	layouts := make(map[string]*paneNode)
	panes := make(map[int]*shellState)
	activeSession := "default"
	var focusPane *paneNode
	nextPaneID := 0
	if sh != nil {
		sh.id = nextPaneID
		nextPaneID++
		panes[sh.id] = sh
		activeSession = sh.session
		focusPane = newLeaf(sh)
		layouts[activeSession] = focusPane
	}
	return model{
		textInput:     ti,
		cmdInput:      ci,
		focus:         paneTerm,
		layouts:       layouts,
		panes:         panes,
		focusPane:     focusPane,
		nextPaneID:    nextPaneID,
		shell:         sh,
		activeSession: activeSession,
		gitBranch:     currentGitBranch(),
		hwMon:         sysmon.New(),
	}
}

// takenOverMsg reports that a newer `ru` displaced this TUI.
type takenOverMsg struct{}

// watchTUITakeover waits on the registration connection; it resolves when the
// daemon displaces this TUI in favor of a newer one. Any other error (daemon
// gone, normal shutdown closing the conn) resolves to a discarded nil msg.
func watchTUITakeover(c *client.Client) tea.Cmd {
	return func() tea.Msg {
		for {
			msg, err := c.Recv()
			if err != nil {
				return nil
			}
			if msg.Type == protocol.MsgTUITakenOver {
				return takenOverMsg{}
			}
		}
	}
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{fetchTreeData, tickCmd()}
	if m.tuiWatch != nil {
		cmds = append(cmds, watchTUITakeover(m.tuiWatch))
	}
	for _, sh := range m.panes {
		go sh.readLoop()
		cmds = append(cmds, waitForPTYOutput(sh.id, sh.ptyCh))
	}
	// `ru --jobs` starts directly in the job view; seed its status bar at once.
	if m.viewMode == viewJobs {
		cmds = append(cmds, sampleHWCmd(m.hwMon))
	}
	return tea.Batch(cmds...)
}

func fetchTreeData() tea.Msg {
	data, err := client.TreeData()
	if err != nil {
		return treeDataMsg{err: err}
	}
	// The pane list is a second round trip, and a nice-to-have next to the
	// tree: if it fails, the Agents tab goes empty rather than the whole
	// refresh reporting an error and blanking the screen.
	terminals, _ := client.ListTerminals()
	return treeDataMsg{
		groups:       data.Groups,
		sessions:     data.Sessions,
		jobs:         data.Jobs,
		terminals:    terminals,
		maxSlots:     data.MaxSlots,
		openJobsView: data.OpenJobsView,
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

type hwStatsMsg sysmon.Stats

// sampleHWCmd reads hardware metrics off the Update goroutine (the macOS path
// shells out to vm_stat) and delivers them as a message.
func sampleHWCmd(mon *sysmon.Monitor) tea.Cmd {
	return func() tea.Msg {
		return hwStatsMsg(mon.Sample())
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// The frame rendered after the message that set it has carried the bell by
	// now, so retire it here: that is what rings it exactly once. Done before
	// the switch so every path, including the early returns, clears it.
	m.clearBell()
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.viewMode == viewSplit && m.termSidebarWidth() == 0 {
			m.jobs.tree.focus = false
		}
		m.resizeShell()
		cmds := []tea.Cmd{clearScreenCmd()}
		// Enable mouse capture the first time we know the size while on the
		// management screen (enabling from Init is unreliable).
		if !m.mouseOn {
			m.mouseOn = true
			cmds = append(cmds, tea.EnableMouseCellMotion)
		}
		return m, tea.Batch(cmds...)

	case treeDataMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		expanded := make(map[string]bool)
		for _, n := range m.nodes {
			expanded[n.group] = n.expanded
		}
		m.groups = msg.groups
		m.agents = msg.terminals
		(&m).updateAlarm(msg.terminals)
		spinCmd := (&m).syncSpinner()
		m.nodes = buildTree(msg.groups, msg.sessions, msg.jobs)
		for i, n := range m.nodes {
			if exp, ok := expanded[n.group]; ok {
				m.nodes[i].expanded = exp
			}
		}
		m.maxSlots = msg.maxSlots
		// A session just created via the management form: open it now that it
		// exists so the user lands in the session they made.
		if m.pendingOpen != "" && m.sessionExists(m.pendingOpen) {
			name := m.pendingOpen
			m.pendingOpen = ""
			nm, cmd := m.openSession(name)
			return nm, tea.Batch(cmd, spinCmd, clearScreenCmd(), tea.EnableMouseCellMotion)
		}
		// Another client (`ru --jobs` from inside a pane) asked us to open the
		// jobs view. Honour it only from the split view so we don't disturb a
		// pager or an already-open table.
		if msg.openJobsView && m.viewMode == viewSplit {
			m, mouseCmd := m.openJobsView()
			return m, tea.Batch(clearScreenCmd(), sampleHWCmd(m.hwMon), mouseCmd, spinCmd)
		}
		m.refreshTreeRows()
		tailCmd := (&m).requestAgentSnippet()
		if tailCmd != nil {
			spinCmd = tea.Batch(spinCmd, tailCmd)
		}
		if m.jobs.session != "" && !m.sessionExists(m.jobs.session) {
			m.jobs.session = ""
		}
		if m.viewMode == viewJobs {
			m.jobs.allJobs = msg.jobs
			m.refreshJobsRows()
			if m.jobs.mode == jobsPager && m.jobs.pager.follow && m.jobs.pager.running {
				return m, tea.Batch(m.reloadPagerCmd(), spinCmd)
			}
		}
		return m, spinCmd

	case pagerLoadedMsg:
		return m.applyPagerLoaded(msg), nil

	case agentTailMsg:
		(&m).applyAgentTail(msg)
		return m, nil

	case spinnerTickMsg:
		m.spinnerFrame++
		// Stop as soon as nothing is working: an idle dashboard should wake on
		// its poll and nothing else.
		if !m.anyWorking() {
			m.spinnerOn = false
			return m, nil
		}
		return m, spinnerTickCmd()

	case hwStatsMsg:
		m.hwStats = sysmon.Stats(msg)
		return m, nil

	case tickMsg:
		m.gitBranch = m.focusedBranch()
		cmds := []tea.Cmd{fetchTreeData, tickCmd()}
		// The hardware status bar only appears in the job view, so only sample
		// while it is open.
		if m.viewMode == viewJobs {
			cmds = append(cmds, sampleHWCmd(m.hwMon))
		}
		return m, tea.Batch(cmds...)

	case ptyOutputMsg:
		if sh := m.panes[msg.id]; sh != nil {
			cmd := waitForPTYOutput(msg.id, sh.ptyCh)
			if msg.clear {
				return m, tea.Batch(clearScreenCmd(), cmd)
			}
			return m, cmd
		}
		return m, nil

	case shellExitMsg:
		return m.handleShellExit(msg.id)

	case ctrlWTimeoutMsg:
		if msg.id == m.ctrlWTimer && m.ctrlWPressed {
			m.ctrlWPressed = false
			// In the MANAGE view the chord only moves tree/list focus; there is no
			// shell to receive a stray Ctrl+W, so only forward it in the split view.
			if m.viewMode == viewSplit && m.focus == paneTerm && m.shell != nil {
				m.shell.write([]byte{0x17})
			}
		}
		return m, nil

	case ctrlBTimeoutMsg:
		return m, nil

	case jobsGTimeoutMsg:
		if msg.id == m.jobs.pendingTimer {
			m.jobs.pending = 0
		}
		return m, nil

	case takenOverMsg:
		m.takenOver = true
		return m, tea.Quit

	case settingsMsg:
		m = m.openSettingsForm(msg.logdir)
		return m, textinput.Blink

	case actionDoneMsg:
		if msg.renamedFrom != "" && msg.renamedFrom != msg.renamedTo {
			m.renameLocalSession(msg.renamedFrom, msg.renamedTo)
		}
		if msg.err != nil {
			m.status = fmt.Sprintf("error: %v", msg.err)
		} else {
			m.status = msg.status
		}
		if msg.offerDiscard != "" {
			m.jobs.confirm = confirmState{
				kind:    confirmDiscardSession,
				session: msg.offerDiscard,
				detail:  msg.detail,
			}
		}
		return m, fetchTreeData

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		if m.viewMode == viewJobs {
			return m.handleJobsKey(msg)
		}
		if m.inputMode == inputCommand {
			return m.handleCommandKey(msg)
		}
		var handled bool
		var cmd tea.Cmd
		m, handled, cmd = m.handleCtrlB(msg)
		if handled {
			return m, cmd
		}
		m, handled, cmd = m.handleCtrlW(msg)
		if handled {
			return m, cmd
		}
		if m.jobs.tree.focus || m.jobs.form.active || m.jobs.confirm.kind != confirmNone {
			return m.handleSessionSidebarKey(msg)
		}
		return m.handleTermKey(msg)
	}
	return m, nil
}

// Run opens the interactive TUI on the jobs & sessions management screen.
func Run() error { return run(false) }

// RunJobs opens the management screen in jobs-only mode, where quitting exits
// the program instead of dropping to a session (used by `ru -j`).
func RunJobs() error { return run(true) }

func run(jobsOnly bool) error {
	cfg, _, warnings := config.LoadDetailed()
	applyTheme(cfg.Theme)

	// Home is the full-screen management view. No terminal session is attached
	// until the user opens one from the list, so no shell is built here — this
	// also means running `ru` from inside an existing runic pane never
	// re-attaches to (or nests on) the pane it was launched from.
	m := newModel(nil)
	m.activeSession = ""
	m.jobsOnly = jobsOnly
	m.agentKinds = cfg.AgentKinds()
	// The Sessions pane starts open, list focused: the three-pane screen is
	// the home view. `,n` still folds it away.
	m.jobs.tree.show = true
	m, _ = m.openJobsView()
	// A rejected theme otherwise only shows up in `ru config list`; the alt
	// screen would hide a stderr line, so say it in the status bar instead.
	for _, w := range warnings {
		if strings.Contains(w, "invalid theme ") {
			m.status = w
		}
	}

	// Register as the daemon's one active TUI. If another `ru` is already
	// attached, the daemon detaches it in our favor; if we get displaced
	// later, tuiWatch delivers takenOverMsg and we quit with a notice.
	tuiConn, err := client.AttachTUI()
	if err == nil {
		defer tuiConn.Close()
		m.tuiWatch = tuiConn
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if fm, ok := finalModel.(model); ok {
		fm.closeShells()
		if fm.takenOver {
			fmt.Fprintln(os.Stderr, "ru: detached — another ru attached to this daemon")
		}
	}
	return err
}

func (m model) closeShells() {
	closed := make(map[*shellState]bool)
	for _, sh := range m.panes {
		if sh != nil && !closed[sh] {
			sh.close()
			closed[sh] = true
		}
	}
}
