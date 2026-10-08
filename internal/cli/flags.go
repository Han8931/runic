package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/han/runic/internal/protocol"
)

func parseShortFlags(cmd *Command, args []string, i *int, actionSet *bool) error {
	arg := args[*i]
	flags := arg[1:]

	for fi := 0; fi < len(flags); fi++ {
		flag := flags[fi]
		switch flag {
		case 'K':
			cmd.Action = ActionKillServer
			*actionSet = true
		case 'T':
			cmd.Action = ActionKillAll
			*actionSet = true
		case 'l':
			cmd.Action = ActionList
			*actionSet = true
		case 'h':
			cmd.Action = ActionShowHelp
			*actionSet = true
		case 'V':
			cmd.Action = ActionShowVersion
			*actionSet = true
		case 'C':
			cmd.Action = ActionClearFinished
			*actionSet = true
		case 'R':
			cmd.Action = ActionCountRunning
			*actionSet = true
		case 'q':
			cmd.Action = ActionLastID
			*actionSet = true
		case 'S':
			cmd.Action = ActionInteractive
			*actionSet = true
		case 'j':
			cmd.Action = ActionJobsView
			*actionSet = true
		case 'n':
			cmd.StoreOutput = false
		case 'E':
			cmd.SeparateStderr = true
		case 'z':
			cmd.GzipOutput = true
		case 'm':
			*i++
			if *i < len(args) {
				cmd.Message = args[*i]
			}
		case 'f':
			cmd.Action = ActionForeground
			*actionSet = true
		case 'B':
			cmd.NonBlocking = true
		case 'd':
			cmd.DependOn = append(cmd.DependOn, -1)
		case 'k':
			cmd.Action = ActionKillJob
			*actionSet = true
			cmd.JobID = optionalJobID(args, i)
		case 'c':
			cmd.Action = ActionCatOutput
			*actionSet = true
			cmd.JobID = optionalJobID(args, i)
		case 'o':
			cmd.Action = ActionShowOutputFile
			*actionSet = true
			cmd.JobID = optionalJobID(args, i)
		case 't':
			cmd.Action = ActionTail
			*actionSet = true
			cmd.JobID = optionalJobID(args, i)
		case 'p':
			cmd.Action = ActionShowPID
			*actionSet = true
			cmd.JobID = optionalJobID(args, i)
		case 'i':
			cmd.Action = ActionInfo
			*actionSet = true
			cmd.JobID = optionalJobID(args, i)
		case 'r':
			cmd.Action = ActionRerun
			*actionSet = true
			cmd.JobID = optionalJobID(args, i)
		case 'x':
			cmd.Action = ActionRemoveJob
			*actionSet = true
			cmd.JobID = optionalJobID(args, i)
		case 'w':
			cmd.Action = ActionWaitJob
			*actionSet = true
			cmd.JobID = optionalJobID(args, i)
		case 'u':
			cmd.Action = ActionUrgent
			*actionSet = true
			cmd.JobID = optionalJobID(args, i)
		case 's':
			cmd.Action = ActionGetState
			*actionSet = true
			cmd.JobID = optionalJobID(args, i)
		case 'a':
			cmd.Action = ActionGetLabel
			*actionSet = true
			cmd.JobID = optionalJobID(args, i)
		case 'F':
			cmd.Action = ActionShowCmd
			*actionSet = true
			cmd.JobID = optionalJobID(args, i)
		case 'g':
			*i++
			if *i >= len(args) {
				return fmt.Errorf("-g requires a session name")
			}
			cmd.Session = args[*i]
		case 'L':
			*i++
			if *i >= len(args) {
				return fmt.Errorf("-L requires a label")
			}
			cmd.Label = args[*i]
		case 'O':
			*i++
			if *i >= len(args) {
				return fmt.Errorf("-O requires a file path")
			}
			cmd.Logfile = args[*i]
		case 'N':
			*i++
			if *i >= len(args) {
				return fmt.Errorf("-N requires a slot count")
			}
			n, err := strconv.Atoi(args[*i])
			if err != nil {
				return fmt.Errorf("invalid slot count: %s", args[*i])
			}
			cmd.NumSlots = n
		case 'P':
			if *i+1 < len(args) {
				if n, err := strconv.Atoi(args[*i+1]); err == nil {
					cmd.Action = ActionSetMaxSlots
					cmd.Slots = n
					*actionSet = true
					*i++
				} else {
					cmd.Action = ActionGetMaxSlots
					*actionSet = true
				}
			} else {
				cmd.Action = ActionGetMaxSlots
				*actionSet = true
			}
		case 'D':
			*i++
			if *i >= len(args) {
				return fmt.Errorf("-D requires a dependency list")
			}
			ids, err := parseIDList(args[*i])
			if err != nil {
				return fmt.Errorf("invalid dependency list: %s", args[*i])
			}
			cmd.DependOn = append(cmd.DependOn, ids...)
		case 'W':
			*i++
			if *i >= len(args) {
				return fmt.Errorf("-W requires a dependency list")
			}
			ids, err := parseIDList(args[*i])
			if err != nil {
				return fmt.Errorf("invalid dependency list: %s", args[*i])
			}
			cmd.DependOn = append(cmd.DependOn, ids...)
			cmd.RequireElevel = true
		case 'U':
			cmd.Action = ActionSwapJobs
			*actionSet = true
			*i++
			if *i < len(args) {
				parts := strings.SplitN(args[*i], "-", 2)
				if len(parts) == 2 {
					id1, err1 := strconv.Atoi(parts[0])
					id2, err2 := strconv.Atoi(parts[1])
					if err1 == nil && err2 == nil {
						cmd.JobID = id1
						cmd.JobID2 = id2
					}
				}
			}
		case 'M':
			*i++
			if *i >= len(args) {
				return fmt.Errorf("-M requires a format")
			}
			switch args[*i] {
			case "json":
				cmd.ListFormat = protocol.FormatJSON
			case "tab":
				cmd.ListFormat = protocol.FormatTab
			default:
				cmd.ListFormat = protocol.FormatDefault
			}
			if !*actionSet {
				cmd.Action = ActionList
				*actionSet = true
			}
		default:
			return fmt.Errorf("unknown flag: -%c", flag)
		}

		// For flags that consume the next arg and break multi-flag parsing
		if flag == 'g' || flag == 'L' || flag == 'O' || flag == 'N' || flag == 'P' ||
			flag == 'D' || flag == 'W' || flag == 'U' || flag == 'M' || flag == 'm' {
			break
		}
	}

	*i++
	return nil
}

func parseLongFlag(cmd *Command, args []string, i *int, actionSet *bool) error {
	arg := args[*i]

	switch {
	case arg == "--getenv":
		cmd.Action = ActionGetEnv
		*actionSet = true
		*i++
		if *i >= len(args) {
			return fmt.Errorf("--getenv requires a key")
		}
		cmd.EnvKey = args[*i]
	case arg == "--setenv":
		cmd.Action = ActionSetEnv
		*actionSet = true
		*i++
		if *i >= len(args) {
			return fmt.Errorf("--setenv requires KEY=VALUE")
		}
		parts := strings.SplitN(args[*i], "=", 2)
		cmd.EnvKey = parts[0]
		if cmd.EnvKey == "" {
			return fmt.Errorf("--setenv requires a key")
		}
		if len(parts) == 2 {
			cmd.EnvValue = parts[1]
		}
	case arg == "--unsetenv":
		cmd.Action = ActionUnsetEnv
		*actionSet = true
		*i++
		if *i >= len(args) {
			return fmt.Errorf("--unsetenv requires a key")
		}
		cmd.EnvKey = args[*i]
	case arg == "--get_logdir":
		cmd.Action = ActionGetLogdir
		*actionSet = true
	case arg == "--set_logdir":
		cmd.Action = ActionSetLogdir
		*actionSet = true
		*i++
		if *i >= len(args) {
			return fmt.Errorf("--set_logdir requires a path")
		}
		cmd.LogdirPath = args[*i]
	case arg == "--timeout":
		*i++
		if *i >= len(args) {
			return fmt.Errorf("--timeout requires a duration (e.g. 30m, 90s)")
		}
		d, err := time.ParseDuration(args[*i])
		if err != nil || d <= 0 {
			return fmt.Errorf("invalid timeout %q (e.g. 30m, 90s)", args[*i])
		}
		cmd.TimeoutMS = d.Milliseconds()
	case arg == "--retries":
		*i++
		if *i >= len(args) {
			return fmt.Errorf("--retries requires a count")
		}
		n, err := strconv.Atoi(args[*i])
		if err != nil || n < 0 {
			return fmt.Errorf("invalid retry count %q", args[*i])
		}
		cmd.Retries = n
	case arg == "--session":
		*i++
		if *i >= len(args) {
			return fmt.Errorf("--session requires a name")
		}
		cmd.Session = args[*i]
	case arg == "--jobs":
		cmd.Action = ActionJobsView
		*actionSet = true
	case arg == "--interactive" || arg == "--tui":
		cmd.Action = ActionInteractive
		*actionSet = true
	case arg == "--slots":
		*actionSet = true
		if n, ok := optionalSlots(args, i); ok {
			cmd.Action = ActionSetMaxSlots
			cmd.Slots = n
		} else {
			cmd.Action = ActionGetMaxSlots
		}
	case arg == "--serialize":
		*i++
		if *i >= len(args) {
			return fmt.Errorf("--serialize requires a format")
		}
		switch args[*i] {
		case "json":
			cmd.ListFormat = protocol.FormatJSON
		case "tab":
			cmd.ListFormat = protocol.FormatTab
		default:
			cmd.ListFormat = protocol.FormatDefault
		}
		if !*actionSet {
			cmd.Action = ActionList
			*actionSet = true
		}
	default:
		return fmt.Errorf("unknown flag: %s", arg)
	}

	*i++
	return nil
}

func optionalJobID(args []string, i *int) int {
	if *i+1 < len(args) {
		if id, err := strconv.Atoi(args[*i+1]); err == nil {
			*i++
			return id
		}
	}
	return -1
}

func optionalSlots(args []string, i *int) (int, bool) {
	if *i+1 < len(args) {
		if n, err := strconv.Atoi(args[*i+1]); err == nil {
			*i++
			return n, true
		}
	}
	return 0, false
}
