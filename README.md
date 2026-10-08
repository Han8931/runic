# Runic

**A terminal workspace for commands, sessions, and coding agents.**

Runic brings a job queue and persistent shell sessions into one terminal interface.
Queue builds and tests, split shells into panes, and give each coding agent its own
Git worktree. Close the interface and return later: the background daemon keeps
your jobs and shells running, while agents can report when they need your attention.

The command is `ru`. Run it without arguments to open the interface, or pass a
command to queue a job. The daemon starts automatically on first use.

[Quick start](#quick-start) · [CLI reference](#usage) · [Terminal interface](#interactive-tui-ru) · [Coding agents](#managing-coding-agents) · [Configuration](#configuration)

## Features

- **Job scheduling** — queue commands with configurable concurrency, dependencies,
  timeouts, and retries. Follow output live or inspect it later.
- **Persistent sessions** — organize shells and jobs into named sessions and groups.
  Split panes, detach, and reconnect without stopping your work.
- **Agent attention** — let agents report `working`, `waiting`, `done`, or `error`
  with `ru mark`. Sessions needing attention rise to the top of their group.
- **Worktree isolation** — give a session its own Git worktree and branch for
  independent changes in the same repository.
- **Keyboard-driven interface** — manage jobs with Vim-style navigation, tmux-style
  pane controls, searchable output, and command-line completion.
- **CLI and configuration** — use JSON or tab-delimited output in scripts, configure
  completion and attention hooks, and retain queue state and settings across daemon restarts.

Runic is written in Go and inspired by
[task-spooler](https://github.com/justanhduc/task-spooler). Job execution supports
Linux, macOS, and Windows; interactive shell panes require Linux or macOS.

## Installation

Build from a checkout with **Go 1.26.1 or later**:

```bash
git clone https://github.com/han/runic.git
cd runic
go install ./cmd/ru/
```

Add Go's binary directory (`GOBIN`, or `$(go env GOPATH)/bin` by default) to your
`PATH`. Install from the checkout because the module uses a bundled terminal
emulator through a local `replace` directive.

With Make installed, you can also use:

```bash
make build      # build ./ru and retire an older daemon if idle
make install    # install ru to GOBIN or GOPATH/bin and check the daemon
make uninstall  # remove ru from GOPATH/bin
```

## Quick start

```bash
# Submit a job
ru echo "hello world"
# 0

# Submit another
ru sleep 10
# 1

# List all jobs (to stdout)
ru -l
# Running: 1/1
# ID   STATE        TIME    COMMAND
# 0    finished     0s      echo hello world
# 1    running      3s      sleep 10

# Open the interactive management TUI (bare `ru`, no args)
ru

# View output of a finished job
ru -c 0
# hello world

# Tail a running job's output
ru -t 1

# Wait for a job to finish
ru -w 1
```

## Usage

```
ru [action] [-nEf] [-m <msg>] [-L <label>] [-D <id,...>] [-W <id,...>]
            [-N <num>] [-O <file>] [-g <session>] [--timeout <dur>]
            [--retries <n>] [cmd...]
```

### Actions

| Flag | Description |
|------|-------------|
| *(default)* | Open the interactive management TUI (jobs & sessions) |
| `-l` | List all jobs to stdout |
| `[cmd...]` | Queue a command |
| `-t [id]` | Tail output of a job (follows live) |
| `-c [id]` | Show output of a job (follows running jobs until they finish; shows exit code if non-zero) |
| `-o [id]` | Show output file path |
| `-p [id]` | Show PID of a running job |
| `-i [id]` | Show detailed info about a job |
| `-s [id]` | Show state of a job |
| `-r [id]` | Rerun a job (re-enqueue the same command; prints the new id) |
| `-x [id]` | Remove a job / delete it from the queue |
| `-w [id]` | Wait for a job to finish |
| `-k [id]` | Kill a running job. The kill is recorded, so a job with `--retries` left is not restarted by it, and the job reports `canceled` rather than a signal death |
| `-u [id]` | Make a queued job urgent (move to front) |
| `-T` | Kill all running jobs |
| `-U <id-id>` | Swap two jobs in the queue |
| `-P [num]`, `--slots [num]` | Set or show max simultaneous slots |
| `-a [id]` | Show job label |
| `-F [id]` | Show full command |
| `-q` | Show last queued job ID |
| `-R` | Count running jobs |
| `-C` | Clear finished jobs |
| `-K` | Kill the server |
| `-S`, `tui` | Open the interactive management TUI (same as bare `ru`) |
| `-j`, `--jobs`, `tui -j` | Open the TUI directly in jobs-only mode (`q` quits) |
| `term ls` | List live interactive panes (`session  pane`) hosted by the daemon |
| `term kill <session> <pane>` | Kill a persistent interactive pane |
| `config …` | Show/edit settings with provenance (see [Configuration](#configuration)) |
| `gc` | Delete orphaned job-output files from the log directory |
| `upgrade` | Stop a stale daemon from an older binary — only if it's idle |

### Job submission modifiers

| Flag | Description |
|------|-------------|
| `-n` | Don't store output |
| `-E` | Separate stderr into `.e` file |
| `-f` | Run in foreground (wait for completion) |
| `-m <message>` | Attach a message to the job (shown by `-i` and as a header in `-c`) |
| `-L <label>` | Assign a label |
| `-N <num>` | Require num slots |
| `-O <file>` | Custom log filename |
| `-d` | Depend on the last queued job |
| `-D <id,...>` | Depend on specific job(s) |
| `-W <id,...>` | Depend on specific job(s) succeeding |
| `-g <session>` | Assign job to a session (default: `"default"`) |
| `--session <s>` | Same as `-g` |
| `--timeout <dur>` | Kill the job after this wall-clock time (e.g. `30m`, `90s`); default: none. Shown in the TUI's TIMEOUT column and editable per job in the `e` box |
| `--retries <n>` | Re-run the job up to `n` extra times when it fails (non-zero exit, signal, or timeout). Attempts append to one output file; `ru -w` resolves on the final attempt. A job you kill yourself is not a failure and is never retried |

### Sessions

```bash
# List sessions
ru session list

# Create a session
ru session create build

# Submit a job to a session
ru -g build make all

# List jobs in a session only
ru -g build -l

# Rename and group a session
ru session rename build ci
ru group create work
ru session move ci work
ru group list

# Delete a session after its jobs finish
ru session delete ci
# For a worktree session, add --discard to discard uncommitted work
```

### Interactive TUI (`ru`)

Run `ru` to open the management screen. The job table shows commands across all
sessions, with group, session, state, timeout, and name columns. A detail pane
shows the selected job, and a status bar tracks CPU, memory, and system load.
The session sidebar provides a collapsible group → session → job tree.

The job table looks like this:

```
╭──────────────────────────────────────────────────────────────────────────────╮
│   ID   GROUP ▲  SESSION  STATE     TIME  TIMEOUT  NAME       COMMAND         │
│    0   default  build    finished  40s   -        compile    make all        │
│    1   default  build    running   4s    30m      tests      make test       │
│    2   default  build    retry 1/2       -        make test  make test       │
│  ·     work     deploy   no jobs                             — empty session │
│ ── job details ────────────────────────────────────────────────────────────  │
│ Command: make all   State: finished   Real time: 40s                         │
╰──────────────────────────────────────────────────────────────────────────────╯
 MANAGE  sort:group▲               run 1 · queue 1 · done 1 · sess 3   ? help
 HW  CPU 4% ▂░░░░░░░  MEM 2.6G/15.0G ▂░░░░░░░    load 1.11 1.35 1.13 · 12 cpu
```

The **TIMEOUT** column shows each job's wall-clock limit (`-` = none) and the
**NAME** column its name — falling back to a dim copy of the command when the
job is unnamed.

Press `Enter` on a job to open its output in the pager. Empty sessions have a
placeholder row; `Enter` opens their shells. Use the session picker (`S`) to
open any session, and `Ctrl+B d` to return to the management screen.

The status bar shows session and job counts and the current mode: `MANAGE`
(the table), `INSERT` (a shell pane), or `COMMAND` (a command prompt). Inside a
session, it also shows the focused pane's Git branch.

Bare `ru` and `ru -S` both open this screen; `ru -j` opens it in **jobs-only**
mode, and `ru -l` still just prints the job list to stdout. `q` quits from this
screen (daemon-hosted shells keep running); `Esc` returns to the open session.

Only one interactive `ru` runs per daemon (like `tmux attach -d`): starting a
second one takes over — the earlier instance exits with a "detached" notice
instead of both mirroring the same shells and fighting over their sizes.
Running `ru` from *inside* a runic pane doesn't take over; it surfaces the
already-running TUI's management screen. Mouse clicks select rows on this screen (mouse is
released inside panes so terminal text-selection still works). Press `?` any time
for a key/command cheatsheet.

**Management key bindings:**

| Key | Action |
|-----|--------|
| `j`/`k`, `gg`/`G`, `Ctrl+d`/`Ctrl+u` | Move / jump / half-page |
| `Space` | Select the cursor job and move down (ranger/lf-style); selected rows stay highlighted, `Esc` clears |
| `Enter` | Open the cursor job's **output** (the pager); on an empty-session row, open that session's shell panes |
| `o` | Open the output pager (scrollable; follows running jobs); press `i` there to overlay job info |
| `S` | Open the session picker, including empty sessions |
| `,n` | Toggle the session sidebar |
| `Tab` | Switch focus between the sidebar and job table |
| `h` / `l` | Collapse / toggle a node in the sidebar |
| `m` / `A` | Open the sidebar action menu / toggle sidebar zoom |
| `e` | Edit the cursor row in one box — a job row: job name, its session's name/group, and its timeout (empty = none); a session row: name + group |
| `n` / `a` | Create a new session (opens the edit box; you land in it) |
| `sg` / `si` / `ss` / `st` | Sort by group / id / state / time; the same chord again (or `R`) reverses |
| `/` | Filter by command/label/session (`Enter` apply, `Esc` cancel) |
| `:` | Command line (see below) |
| `?` | Show the help overlay |
| `V` | Toggle visual mode; `j`/`k` extend the selection |
| `x` / `u` / `r` | Kill / make urgent / rerun the selected job(s) — the Space-selected set, the visual range, or the cursor row |
| `d` | Remove the selected job(s); finished jobs delete immediately, others confirm `y`/`n` |
| `D` / `C` | Delete all finished jobs (no confirm / confirm `y`/`n`) |
| `q` | Quit (daemon-hosted shells keep running) |
| `Esc` | Back to the open session (quits if none is open) |

#### Command line (`:`)

Press `:` to open a command line on the management screen:

| Command | Action |
|---------|--------|
| `:set slots <n>` (or `:slots <n>`) | Set the number of parallel job slots |
| `:set logdir <path>` | Set the daemon's log directory |
| `:sort <group\|id\|state\|time>` | Set the sort field |
| `:config` | Open the settings edit box (slots, log directory) |
| `:kill <id…>` / `:kill -a` | Kill the given jobs (no id: current selection) / all running jobs |
| `:restart <id…>` / `:restart -a` | Restart job(s): kill if running, then re-enqueue (no id: selection; `-a`: every non-queued job) |
| `:reset` | Factory-reset runic — kills all jobs & panes, deletes sessions/groups, restores default settings (asks `y`/`n`) |
| `:clear` | Clear finished jobs |
| `:help` | Show the help overlay |
| `:q` | Quit |

#### Sessions & the session picker

A **session** is a named workspace with its own shells and its own jobs; sessions
are organized into **groups**. Every session shows in the table (empty ones as a
placeholder row, which `Enter` opens directly). The **session picker** (`S`)
gives a compact grouped list of every session — the way to open any session,
and handy for jumping around or reaching a session you just made:

```
╭─ Sessions ───────────────────────────────╮
│    default                               │
│  ▌ build             1 jobs              │
│    deploy            2 jobs              │
│    work                                  │
│    idle              0 jobs              │
│  ⏎:open · e:edit · n:new · d:delete · esc │
╰───────────────────────────────────────────╯
```

`e` (edit) and `n` (new) open a small modal to set a session's **name** and
**group**; submitting renames/moves or creates it. While the group field is
focused, the existing groups are listed in the box with the current match
highlighted — `Tab` steps through them, or type a new name to create a group
on save. On a **group header** row, `e` renames the group and `d` deletes it
(empty groups only, with confirmation).

#### Panes (tmux-style splits)

Inside a session the terminal can be split into multiple shells, tiled like tmux.
Each split spawns a fresh shell for that session; splits can be nested arbitrarily.
Commands are reachable two ways: the `Ctrl+B` prefix (press `Ctrl+B`, release,
then the key) and command mode (`Ctrl+B` then `c`, type the command, `Enter`).

The shells run inside the background daemon, so they **persist**: detach with
`Ctrl+B d` (or quit and reopen `ru`) and your panes, their layout, and any
still-running processes are reattached (recent scrollback is replayed). Panes
live until you close them (`Ctrl+B x`) or the daemon is stopped (`ru -K`).

| Prefix | Command mode | Action |
|--------|--------------|--------|
| `Ctrl+B` `\|` / `%` | `vs` / `vsplit` | Split focused pane vertically (side by side) |
| `Ctrl+B` `-` / `"` | `hs` / `hsplit` / `split` | Split focused pane horizontally (stacked) |
| `Ctrl+B` `o` | `o` / `next` | Cycle focus to the next pane |
| `Ctrl+B` arrows / `h` `k` `l` | — | Move focus to the adjacent pane |
| `Ctrl+W` then `h`/`j`/`k`/`l` / `w` | — | Move / cycle focus between panes |
| `Ctrl+B` `x` / `Ctrl+W` `q` | `x` / `close` | Close the focused pane (last pane is kept) |
| `Ctrl+B` `d` | `detach` | Detach back to the management screen (shells persist) |
| `Ctrl+B` `j` | `jobs` / `j` | Jump to the management screen |
| `Ctrl+B` `c` | — | Open command mode |
| `Ctrl+B` `q` | `q` / `quit` | Quit |

The `Ctrl+B` prefix stays active until the next key. Press `Esc` after `Ctrl+B`
to cancel it. If the next key isn't a recognized chord, the buffered `Ctrl+B`
is forwarded to the focused shell, so shell shortcuts still work.

#### Command-line completion

Both command lines — the management screen's `:` and the pane view's `Ctrl+B c`
— complete with `Tab`, following the shell convention:

| Key | Action |
|-----|--------|
| `Tab` | Complete as far as unambiguous; list the candidates when it can't |
| `Tab` again | Cycle forward through the candidates |
| `Shift+Tab` | Cycle backward |

Candidates appear in a strip under the input, with the current one highlighted
while cycling. Completion is context-aware: command names at the start of the
line, then that command's own values — `:sort <Tab>` offers the sort modes,
`:kill <Tab>` offers `--all`, and `:set <Tab>` offers the settings and then
*their* values (`:set sort <Tab>` → the sort modes). `:set logdir <Tab>`
completes directory paths off the filesystem, `~/` included.

On an empty word, `Tab` lists without inserting, so it never pre-fills a command
you didn't ask for; the next `Tab` commits to the first candidate. Only spellings
the command line actually accepts are ever suggested.

Activating a session sets `RUNIC_SESSION` in the embedded shell; `ru` uses it as
the default session when `-g` is omitted. Running `ru` (or `ru -j`) from *inside*
a pane won't nest a second TUI — it surfaces the already-running management screen
instead, so you never open a session inside a session. The embedded shell supports
vi mode (`set -o vi`) — all keystrokes are forwarded directly.

In the output pager: `j`/`k`, `gg`/`G`, `Ctrl+d`/`Ctrl+u` scroll, `/` then
`n`/`N` search, `i` toggles a job-info panel, `G` re-follows a running job,
`q`/`Esc` returns to the table.

### Examples

```bash
# Run 3 jobs in parallel
ru -P 3        # or: ru --slots 3

# Submit a labeled job
ru -L "build" make all

# Attach a message (like a git commit message)
ru -m "trying with -O3" make all
# ru -i 0 shows: Message: trying with -O3
# ru -c 0 prepends: # trying with -O3

# Chain jobs with dependencies
ru -L "build" make all        # job 0
ru -d -L "test" make test     # job 1, runs after job 0
ru -D 0,1 -L "deploy" ./deploy.sh  # runs after both 0 and 1

# Only run if dependency succeeded (exit code 0)
ru -W 0 echo "build passed!"

# Run in foreground (blocks until done)
ru -f make build

# JSON output for scripting
ru -M json

# Tab-delimited output
ru -M tab

# Submit jobs to different sessions
ru -g build make all
ru -g test make test

# Open interactive TUI
ru -S

# Kill a job that runs longer than 30 minutes
ru --timeout 30m make test

# Re-run a flaky job up to 2 extra times if it fails
ru --retries 2 ./flaky-test.sh

# Server-side environment variables
ru --setenv MY_VAR=value
ru --getenv MY_VAR

# Change log directory
ru --set_logdir /tmp/my-logs
ru --get_logdir

# Clean up orphaned output files
ru gc
```

## Configuration

Settings are layered; later layers win:

```
built-in defaults  <  config file  <  environment variables  <  runtime overrides
```

The **config file** is `~/.config/runic/config` (`$XDG_CONFIG_HOME` respected)
with plain `key = value` lines and `#` comments. **Runtime overrides** are
recorded automatically when you change a setting on a live daemon (`ru -P`,
`--set_logdir`, or the TUI's `:config` box), so those changes survive daemon
restarts; they outrank the environment because they capture your most recent
explicit choice.

```bash
ru config              # every setting: value + source (default/file/env/runtime)
ru config get slots
ru config set slots 4  # writes the file and applies to a running daemon
ru config edit         # open the file in $EDITOR (creates a template)
ru config path
```

| Key | Environment | Description | Default |
|-----|-------------|-------------|---------|
| `slots` | `RUNIC_SLOTS` | Max simultaneous job slots | `1` |
| `logdir` | `RUNIC_LOGDIR` | Directory for job output files | `$TMPDIR` |
| `socket` | `RUNIC_SOCKET` | Daemon socket path | `$TMPDIR/runic-socket.<uid>` |
| `max_finished` | `RUNIC_MAXFINISHED` | Finished jobs to keep (`-1`: unlimited) | unlimited |
| `max_conn` | `RUNIC_MAXCONN` | Max client connections | `10` |
| `on_finish` | `RUNIC_ONFINISH` | Command to run on job completion | — |
| `on_attention` | `RUNIC_ONATTENTION` | Command to run when a pane's attention state changes | — |
| `save_list` | `RUNIC_SAVELIST` | Queue snapshot file (`none` disables) | `~/.local/state/runic/queue.json` |

`RUNIC_SESSION` (environment only) sets the default session when `-g` is
omitted; the TUI sets it inside session shells. Legacy `TS_*` names are still
accepted as fallbacks for compatibility.

## Managing coding agents

Run coding agents in session panes to keep them running when you close the
interface. Attention states show which agents need input; worktree sessions give
each agent a separate checkout.

### Report attention with `ru mark`

Every pane carries a self-reported **attention state**, so the session list reads
like an inbox instead of a wall of identical shells:

| State | Meaning | In the tree |
|-------|---------|-------------|
| `waiting` | blocked on you | `●` amber, floats to the top, with how long |
| `error` | stopped on a failure | `×` red, floats to the top, with how long |
| `working` | busy, needs nothing | `◐` green |
| `done` / `idle` | nothing to do | plain |

The sidebar shows how long a session has needed attention, such as `14m`.
Select a session to see its branch, worktree, latest note, and job counts in
the detail pane.

A process inside a pane reports its own state. Panes get `RUNIC_SESSION` and
`RUNIC_PANE` in their environment, so no configuration is needed:

```bash
ru mark working                      # from inside a pane
ru mark waiting "which schema?"      # the note shows in the tree
ru mark error "tests failed"
ru mark done
ru mark idle --session agent-1 --pane p2   # or target one explicitly
```

Sessions roll up to their most urgent pane, `N need you` appears on the status
bar, and the sessions needing you sort to the top of their group. The state is
**self-reported on purpose**: a full-screen agent lives in the alt-screen, where
guessing from output is unreliable. Two things adjust it automatically — a shell
prompt clears a stale `working` (whatever was running has exited), and attaching
to a pane clears `waiting`/`error`, since opening it is how you acknowledge it.

Wire it to [Claude Code](https://claude.com/claude-code) hooks in
`.claude/settings.json` so the agent reports itself:

```json
{
  "hooks": {
    "Notification": [
      {"hooks": [{"type": "command", "command": "ru mark waiting \"$(jq -r .message)\" 2>/dev/null || true"}]}
    ],
    "Stop": [
      {"hooks": [{"type": "command", "command": "ru mark done 2>/dev/null || true"}]}
    ],
    "UserPromptSubmit": [
      {"hooks": [{"type": "command", "command": "ru mark working 2>/dev/null || true"}]}
    ]
  }
}
```

The `|| true` keeps the hook harmless when the agent is run outside a runic pane.

Set `on_attention` to be told about transitions even when you aren't looking at
the TUI. The command gets `RUNIC_SESSION`, `RUNIC_PANE`, `RUNIC_ATTENTION`,
`RUNIC_ATTENTION_FROM`, and `RUNIC_NOTE`; it fires on every transition, so filter
on the state you care about:

```bash
ru config set on_attention '[ "$RUNIC_ATTENTION" = waiting ] && notify-send "runic: $RUNIC_SESSION" "$RUNIC_NOTE"'
```

`ru term ls` prints the same states as `session<TAB>pane<TAB>state<TAB>note`.

### Isolate work with worktree sessions

A session can own a fresh `git worktree` on its own branch, so two agents editing
the same repository never see each other's half-finished work:

```bash
cd ~/src/myapp
ru session create agent-1 --worktree               # branch "agent-1"
ru session create agent-2 --branch fix/login       # explicit branch
ru session create agent-3 --worktree --repo ~/src/other
```

In the TUI, the **New session** box (`n`) has a `branch` field: leave it blank
for an ordinary session, or name a branch to isolate it. Isolated sessions show
`⊕` in the tree, and the branch appears in the pane status bar and the tree's
action menu (`m`).

Runic creates worktrees under `~/.local/state/runic/worktrees/` and only removes
worktree directories it manages. Deleting a session keeps its branch and commits.
Panes start in the session's worktree; queued jobs run in the directory from
which they were submitted.

Deleting a session removes its worktree **only if it is clean**. If uncommitted
edits or untracked files remain, Runic refuses deletion and lists the files.
Commit or save that work first, or use `--discard` to delete it:

```console
$ ru session delete agent-1
ru: worktree has uncommitted work: 2 uncommitted changes in
    ~/.local/state/runic/worktrees/agent-1: src/login.go, notes.md

$ cd ~/.local/state/runic/worktrees/agent-1 && git status   # salvage it…
$ ru session delete agent-1 --discard                       # …or throw it away
```

In the TUI, `d` on the session asks a second time, naming what would be lost.
`:reset` always discards: it is the one command that says so in its own prompt.

## Output files

Each job's output is stored in the log directory (default `$TMPDIR`; see the
`logdir` setting) as `ru_<jobID>_<random>.out` (8 random hex chars). The random suffix prevents old log files from being overwritten when job IDs repeat across daemon restarts. Use `ru -o <id>` to print the exact path, `ru -c <id>` to view (or follow) the content.

When a job leaves the queue (`ru -x`, `ru -C`, `RUNIC_MAXFINISHED` pruning, or
deleting its session), its auto-generated output file is deleted with it, so
the log directory doesn't accumulate orphaned files. Files you named yourself
with `-O` are never deleted. `ru gc` sweeps the current log directory for
generated `ru_*.out` files that no live job references (leftovers from older
versions or unpersisted queues) — user-named files are never candidates.

## Architecture

`runic` is a single binary that acts as both client and server:

- **Server**: A background daemon that manages the job queue, executes jobs, and communicates with clients via Unix domain sockets (Linux/macOS) or TCP on localhost (Windows). On Windows, connections must present a random token from the owner-only (0600) port file, so other local users can't drive your daemon.
- **Client**: Connects to the server to submit jobs, query status, and control execution.
- The server starts automatically on first client connection and runs until killed with `ru -K`.
- The daemon logs to `~/.local/state/runic/daemon.log` (`$XDG_STATE_HOME`
  respected; rotated to `.old` past ~1MB) — startup info, config warnings, and
  runtime errors land there, since the detached daemon has no terminal.
- After upgrading `ru`, a daemon from an older binary is **never killed
  automatically**: commands refuse with a version message until you run
  `ru -K` yourself, so running jobs and live panes die only when you decide.
  `ru upgrade` (run by `make build`) automates the safe case: it stops a stale
  daemon only when it has **no running jobs and no live panes**, and otherwise
  leaves it with a note.

## Building

```bash
# Native build
make build

# Run checks
make test       # tests with the race detector
make vet        # static checks

# Cross-compile
GOOS=linux GOARCH=amd64 go build -o ru-linux ./cmd/ru/
GOOS=darwin GOARCH=arm64 go build -o ru-macos ./cmd/ru/
GOOS=windows GOARCH=amd64 go build -o ru.exe ./cmd/ru/
```

## Roadmap

- HW monitoring popup box
- CI (vet + race tests across Linux/macOS/Windows)
- Slots as an agent concurrency/cost governor (cap N agents, queue the rest)
- Auto-enqueue a follow-up job when an agent finishes (run tests → open a PR)

## License

MIT
