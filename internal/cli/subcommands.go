package cli

import (
	"fmt"
	"strings"

	"github.com/han/runic/internal/protocol"
)

func parseSessionSubcommand(cmd *Command, args []string) (*Command, error) {
	if len(args) == 0 {
		cmd.Action = ActionSessionList
		return cmd, nil
	}

	switch args[0] {
	case "list":
		cmd.Action = ActionSessionList
	case "create":
		cmd.Action = ActionSessionCreate
		if len(args) < 2 {
			return nil, fmt.Errorf("session create requires a name")
		}
		cmd.Session = args[1]
		// `--worktree` gives the session its own git worktree and branch, so an
		// agent in it cannot see another session's edits.
		for i := 2; i < len(args); i++ {
			switch args[i] {
			case "--worktree", "-W":
				cmd.Worktree = true
			case "--branch", "-b":
				i++
				if i >= len(args) {
					return nil, fmt.Errorf("--branch requires a name")
				}
				cmd.Worktree = true
				cmd.Branch = args[i]
			case "--repo":
				i++
				if i >= len(args) {
					return nil, fmt.Errorf("--repo requires a path")
				}
				cmd.Worktree = true
				cmd.Repo = args[i]
			default:
				return nil, fmt.Errorf("unknown session create option: %s", args[i])
			}
		}
	case "rename":
		cmd.Action = ActionSessionRename
		if len(args) < 3 {
			return nil, fmt.Errorf("session rename requires old and new names")
		}
		cmd.Session = args[1]
		cmd.SessionArg = args[2]
	case "delete":
		cmd.Action = ActionSessionDelete
		if len(args) < 2 {
			return nil, fmt.Errorf("session delete requires a name")
		}
		cmd.Session = args[1]
		// Deleting a worktree session with uncommitted work is refused unless the
		// loss is asked for by name.
		for i := 2; i < len(args); i++ {
			switch args[i] {
			case "--discard", "--force", "-f":
				cmd.Discard = true
			default:
				return nil, fmt.Errorf("unknown session delete option: %s", args[i])
			}
		}
	case "move":
		cmd.Action = ActionSessionMove
		if len(args) < 3 {
			return nil, fmt.Errorf("session move requires a session and group")
		}
		cmd.Session = args[1]
		cmd.SessionArg = args[2]
	default:
		return nil, fmt.Errorf("unknown session command: %s", args[0])
	}
	return cmd, nil
}

// parseMarkSubcommand handles `ru mark <state> [note...]`, the self-report a
// pane (or an agent hook running in one) uses to say whether it is working,
// waiting on you, done, or broken. The target pane comes from RUNIC_SESSION /
// RUNIC_PANE unless --session / --pane override it.
func parseMarkSubcommand(cmd *Command, args []string) (*Command, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("mark requires a state (working|waiting|done|error)")
	}
	cmd.Action = ActionMark

	state, ok := protocol.ParseAttention(args[0])
	if !ok {
		return nil, fmt.Errorf("unknown mark state %q (want working|waiting|done|error|clear)", args[0])
	}
	cmd.Attention = state

	var note []string
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--session", "-g":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--session requires a name")
			}
			cmd.Session = args[i]
		case "--pane":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--pane requires a name")
			}
			cmd.Pane = args[i]
		default:
			note = append(note, args[i])
		}
	}
	cmd.Message = strings.Join(note, " ")
	return cmd, nil
}

func parseGroupSubcommand(cmd *Command, args []string) (*Command, error) {
	if len(args) == 0 {
		cmd.Action = ActionGroupList
		return cmd, nil
	}

	switch args[0] {
	case "list":
		cmd.Action = ActionGroupList
	case "create":
		cmd.Action = ActionGroupCreate
		if len(args) < 2 {
			return nil, fmt.Errorf("group create requires a name")
		}
		cmd.Session = args[1]
	case "rename":
		cmd.Action = ActionGroupRename
		if len(args) < 3 {
			return nil, fmt.Errorf("group rename requires old and new names")
		}
		cmd.Session = args[1]
		cmd.SessionArg = args[2]
	case "delete":
		cmd.Action = ActionGroupDelete
		if len(args) < 2 {
			return nil, fmt.Errorf("group delete requires a name")
		}
		cmd.Session = args[1]
	default:
		return nil, fmt.Errorf("unknown group command: %s", args[0])
	}
	return cmd, nil
}

func parseTermSubcommand(cmd *Command, args []string) (*Command, error) {
	if len(args) == 0 {
		cmd.Action = ActionTermList
		return cmd, nil
	}
	switch args[0] {
	case "ls", "list":
		cmd.Action = ActionTermList
	case "kill":
		cmd.Action = ActionTermKill
		if len(args) < 3 {
			return nil, fmt.Errorf("term kill requires a session and pane")
		}
		cmd.Session = args[1]
		cmd.SessionArg = args[2]
	default:
		return nil, fmt.Errorf("unknown term command: %s", args[0])
	}
	return cmd, nil
}

// parseTuiSubcommand handles `ru tui` (interactive split view) and
// `ru tui -j` / `ru tui jobs` (open directly in the job-management view).
func parseTuiSubcommand(cmd *Command, args []string) (*Command, error) {
	cmd.Action = ActionInteractive
	if len(args) == 0 {
		return cmd, nil
	}
	switch args[0] {
	case "-j", "--jobs", "jobs":
		cmd.Action = ActionJobsView
	default:
		return nil, fmt.Errorf("unknown tui command: %s", args[0])
	}
	return cmd, nil
}

// parseConfigSubcommand handles `ru config [list|get|set|edit|path]`. The key
// lands in EnvKey and (for set) the value in EnvValue; a multi-word value
// (e.g. an on_finish command) is joined with spaces.
func parseConfigSubcommand(cmd *Command, args []string) (*Command, error) {
	if len(args) == 0 {
		cmd.Action = ActionConfigList
		return cmd, nil
	}
	switch args[0] {
	case "list":
		cmd.Action = ActionConfigList
	case "get":
		if len(args) < 2 {
			return nil, fmt.Errorf("config get requires a key")
		}
		cmd.Action = ActionConfigGet
		cmd.EnvKey = args[1]
	case "set":
		if len(args) < 3 {
			return nil, fmt.Errorf("config set requires a key and a value")
		}
		cmd.Action = ActionConfigSet
		cmd.EnvKey = args[1]
		cmd.EnvValue = strings.Join(args[2:], " ")
	case "edit":
		cmd.Action = ActionConfigEdit
	case "path":
		cmd.Action = ActionConfigPath
	default:
		return nil, fmt.Errorf("unknown config command: %s", args[0])
	}
	return cmd, nil
}
