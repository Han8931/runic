package cli

import (
	"strconv"
	"strings"

	"github.com/han/runic/internal/protocol"
)

type Action int

const (
	ActionQueue Action = iota
	ActionList
	ActionKillServer
	ActionKillAll
	ActionKillJob
	ActionClearFinished
	ActionShowHelp
	ActionShowVersion
	ActionCatOutput
	ActionShowOutputFile
	ActionShowPID
	ActionRemoveJob
	ActionWaitJob
	ActionUrgent
	ActionGetState
	ActionSwapJobs
	ActionInfo
	ActionSetMaxSlots
	ActionGetMaxSlots
	ActionCountRunning
	ActionGetLabel
	ActionLastID
	ActionShowCmd
	ActionTail
	ActionGetEnv
	ActionSetEnv
	ActionUnsetEnv
	ActionGetLogdir
	ActionSetLogdir
	ActionForeground
	ActionServerMode
	ActionInteractive
	ActionSessionList
	ActionSessionCreate
	ActionSessionRename
	ActionSessionDelete
	ActionSessionMove
	ActionGroupList
	ActionGroupCreate
	ActionGroupRename
	ActionGroupDelete
	ActionTermList
	ActionTermKill
	ActionRerun
	ActionJobsView
	ActionConfigList
	ActionConfigGet
	ActionConfigSet
	ActionConfigEdit
	ActionConfigPath
	ActionGC
	ActionUpgrade
	ActionMark
)

type Command struct {
	Action     Action
	Command    []string
	JobID      int
	JobID2     int
	Slots      int
	Label      string
	DependOn   []int
	ListFormat protocol.ListFormat
	EnvKey     string
	EnvValue   string
	LogdirPath string
	Logfile    string

	Session    string
	SessionArg string
	Message    string

	TimeoutMS int64
	Retries   int

	// Attention is the state `ru mark` reports; Pane names the pane it applies
	// to (empty means every pane in the session).
	Attention protocol.Attention
	Pane      string

	// Worktree asks `session create` to give the session its own git worktree;
	// Branch and Repo override the defaults (session name, current directory).
	Worktree bool
	Branch   string
	Repo     string
	// Discard lets `session delete` throw away uncommitted work in the session's
	// worktree. Without it, such a session is refused and its worktree kept.
	Discard bool

	StoreOutput    bool
	SeparateStderr bool
	GzipOutput     bool
	RequireElevel  bool
	NonBlocking    bool
	NumSlots       int
}

func Parse(args []string) (*Command, error) {
	cmd := &Command{
		Action:      ActionList,
		JobID:       -1,
		JobID2:      -1,
		StoreOutput: true,
		NumSlots:    1,
		ListFormat:  protocol.FormatDefault,
	}

	// Bare `ru` opens the interactive management TUI. The plain job listing is
	// available via `ru -l`.
	if len(args) == 0 {
		cmd.Action = ActionInteractive
		return cmd, nil
	}

	if args[0] == "--server" {
		cmd.Action = ActionServerMode
		return cmd, nil
	}

	if args[0] == "session" {
		return parseSessionSubcommand(cmd, args[1:])
	}
	if args[0] == "group" {
		return parseGroupSubcommand(cmd, args[1:])
	}
	if args[0] == "term" {
		return parseTermSubcommand(cmd, args[1:])
	}
	if args[0] == "tui" {
		return parseTuiSubcommand(cmd, args[1:])
	}
	if args[0] == "config" {
		return parseConfigSubcommand(cmd, args[1:])
	}
	if args[0] == "mark" {
		return parseMarkSubcommand(cmd, args[1:])
	}
	if args[0] == "gc" {
		cmd.Action = ActionGC
		return cmd, nil
	}
	if args[0] == "upgrade" {
		cmd.Action = ActionUpgrade
		return cmd, nil
	}

	i := 0
	actionSet := false

	for i < len(args) {
		arg := args[i]

		if strings.HasPrefix(arg, "--") {
			if err := parseLongFlag(cmd, args, &i, &actionSet); err != nil {
				return nil, err
			}
			continue
		}

		if strings.HasPrefix(arg, "-") && len(arg) > 1 {
			if err := parseShortFlags(cmd, args, &i, &actionSet); err != nil {
				return nil, err
			}
			continue
		}

		break
	}

	if i < len(args) && !actionSet {
		cmd.Action = ActionQueue
		cmd.Command = args[i:]
		return cmd, nil
	} else if i < len(args) && actionSet {
		cmd.Command = args[i:]
	}

	return cmd, nil
}

func parseIDList(s string) ([]int, error) {
	parts := strings.Split(s, ",")
	ids := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		id, err := strconv.Atoi(p)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}
