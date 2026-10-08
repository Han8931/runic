package protocol

import (
	"encoding/json"
	"fmt"
	"time"
)

type JobState int

const (
	StateQueued JobState = iota
	StateAllocating
	StateRunning
	StateFinished
	StateSkipped
	StateHoldingClient
)

func (s JobState) String() string {
	switch s {
	case StateQueued:
		return "queued"
	case StateAllocating:
		return "allocating"
	case StateRunning:
		return "running"
	case StateFinished:
		return "finished"
	case StateSkipped:
		return "skipped"
	case StateHoldingClient:
		return "holding_client"
	default:
		return "unknown"
	}
}

func (s JobState) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

func (s *JobState) UnmarshalJSON(data []byte) error {
	var name string
	if err := json.Unmarshal(data, &name); err != nil {
		return fmt.Errorf("job state must be a string: %w", err)
	}
	states := map[string]JobState{
		"queued":         StateQueued,
		"allocating":     StateAllocating,
		"running":        StateRunning,
		"finished":       StateFinished,
		"skipped":        StateSkipped,
		"holding_client": StateHoldingClient,
	}
	state, ok := states[name]
	if !ok {
		return fmt.Errorf("unknown job state %q", name)
	}
	*s = state
	return nil
}

type ListFormat int

const (
	FormatDefault ListFormat = iota
	FormatJSON
	FormatTab
)

type Result struct {
	ExitCode     int
	DiedBySignal bool
	Signal       int
	UserTimeMS   int64
	SystemTimeMS int64
	RealTimeMS   int64
	Skipped      bool
	TimedOut     bool // killed because it exceeded the job's --timeout
	// Canceled means a human stopped the job (`ru -k`, `x` in the TUI). The
	// exit status of a killed process is the same as a crashed one, so this is
	// the only way to tell the two apart — and it is what suppresses the retry
	// a failure would otherwise trigger.
	Canceled bool
}

type JobInfo struct {
	ID             int
	Command        string
	State          JobState
	Result         Result
	OutputFilename string
	StoreOutput    bool
	PID            int
	DependOn       []int
	Label          string
	Session        string
	Message        string
	NumSlots       int
	NumGPUs        int
	GPUIDs         []int
	EnqueueTime    time.Time
	StartTime      time.Time
	EndTime        time.Time
	TimeoutMS      int64 // kill the job after this wall-clock time (0: none)
	Retries        int   // extra attempts allowed after a failure
	Attempt        int   // retries used so far (0 on the first run)
}

// Attention is a pane's self-reported state, set by `ru mark` from inside the
// pane — typically by a coding agent's own hook (Claude Code's Stop /
// Notification hooks, say) rather than guessed from its output. Full-screen
// agents live in the alt-screen, where output-pattern detection is unreliable,
// so the self-report is the primary signal.
//
// The constants are ordered by urgency: a higher value outranks a lower one,
// which is what lets a session roll its panes up with a plain max() and what
// floats "needs you" to the top of the TUI's tree.
type Attention int

const (
	AttentionNone    Attention = iota // nothing reported
	AttentionIdle                     // reported done; nothing to do
	AttentionWorking                  // busy, needs nothing from you
	AttentionError                    // stopped on a failure
	AttentionWaiting                  // blocked waiting on you
)

func (a Attention) String() string {
	switch a {
	case AttentionIdle:
		return "idle"
	case AttentionWorking:
		return "working"
	case AttentionError:
		return "error"
	case AttentionWaiting:
		return "waiting"
	default:
		return "none"
	}
}

// NeedsYou reports whether a state is one a human has to act on. Only these
// float to the top of the TUI tree, so a pane merely working away never
// reshuffles the sidebar under your cursor.
func (a Attention) NeedsYou() bool {
	return a == AttentionWaiting || a == AttentionError
}

// ParseAttention maps a `ru mark` argument to its state. "done" is accepted as
// a synonym for idle, since that is how an agent's finish hook reads.
func ParseAttention(s string) (Attention, bool) {
	switch s {
	case "idle", "done":
		return AttentionIdle, true
	case "working", "busy":
		return AttentionWorking, true
	case "error", "errored", "failed":
		return AttentionError, true
	case "waiting", "wait", "input":
		return AttentionWaiting, true
	case "none", "clear":
		return AttentionNone, true
	}
	return AttentionNone, false
}

func (a Attention) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.String())
}

type SessionInfo struct {
	Name  string
	Group string
	// Attention is the most urgent state across the session's live panes
	// (AttentionNone when it has none). The daemon rolls it up so the TUI gets
	// it in the same tree poll as the rest of the hierarchy.
	Attention Attention
	// AttentionNote is the message that came with that state, if any.
	AttentionNote string
	// AttentionSince is when the reporting pane entered that state, so a client
	// can show how long an agent has been waiting. Zero when nothing is
	// reported — "waiting" with no age reads as "just now", which is a lie once
	// it has been an hour.
	AttentionSince time.Time
	// Worktree is the git worktree directory the session's panes start in, when
	// it owns one (empty otherwise), and Branch is what is checked out there.
	Worktree string
	Branch   string
}

type NewJobRequest struct {
	Command        string
	CommandArgs    []string
	WorkDir        string
	Environment    []string
	StoreOutput    bool
	SeparateStderr bool
	GzipOutput     bool
	DependOn       []int
	RequireElevel  bool
	Label          string
	Session        string
	Message        string
	NumSlots       int
	Logfile        string
	TimeoutMS      int64
	Retries        int
}
