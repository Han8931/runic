# Runic

**A terminal workspace for commands, sessions, and coding agents.**

Runic brings a job queue and persistent shell sessions into one terminal interface.
Queue builds and tests, split shells into panes, and give each coding agent its own
Git worktree. Close the interface and return later: the background daemon keeps
your jobs and shells running, while agents can report when they need your attention.

The command is `ru`. Run it without arguments to open the interface, or pass a
command to queue a job. The daemon starts automatically on first use.

<p align="center">
  <img src="assets/runic_demo.gif" alt="Runic demo" width="800">
</p>

[Quick start](#quick-start) · [CLI reference](#usage) · [Terminal interface](#interactive-tui-ru) · [Coding agents](#managing-coding-agents) · [Configuration](#configuration)

## Features

- **Job scheduling** — queue commands with configurable concurrency, dependencies,
  timeouts, and retries. Follow output live or inspect it later.
- **Persistent sessions** — organize shells and jobs into named sessions and groups.
  Split panes, detach, and reconnect without stopping your work.
- **Agent sessions** — pick an agent (`claude`, `codex`, …) when creating a session
  and runic starts it in the session's first pane, on its own branch if you want one.
- **Agent attention** — let agents report `working`, `waiting`, `done`, or `error`
  with `ru mark`. Sessions needing attention rise to the top of their group, and an
  agent that starts waiting on you rings the bell and names itself on the status bar.
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
| `ack [session] [pane]` | Acknowledge a pane's alert without attaching: it stops counting as needing you, but keeps its state and note |
| `agent hooks [kind]` | Print the hook snippet that makes an agent report itself (sessions started with `--agent` are already instrumented) |
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

# Create a session running a coding agent, isolated on its own branch
ru session create agent-1 --agent claude --worktree

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

Run `ru` to open the management screen. It is laid out as bordered panes: a
**Sessions** sidebar on the left, the **Jobs** table across all sessions, and a
**Details** pane for whatever the cursor is on beneath the table.

Both panes carry view tabs, and `Tab` / `Shift+Tab` roll **the focused pane's**
views — so one gesture always means "next view". Clicking a tab switches to it.

The Sessions pane has two: **Spaces** and **Agents**.

- **Spaces** is the collapsible group → session → agent/job tree. A group *is* a
  space: a named workspace holding sessions. A session's agents are its
  children, marked its kind rather than its session's name — which is
  already on the row above — so an agent row cannot be mistaken for another
  session: a session carries a fold mark, an agent sits a level deeper. An agent
  that needs you sits **above** the session's jobs, and a quiet one sits below
  them, out of the way until it has something to say.
- **Agents** drops the hierarchy and lists every live pane that has reported a
  state, most urgent first — errors, then panes waiting on you, then the ones
  working, then the quiet ones. Within a state the one that has been there
  longest sorts first. The tab carries a `2!` badge for panes needing you, so a
  blocked agent is visible from the Spaces view.

The main pane has two **primary views** — **Agents** and **Jobs** — on one tab
strip, with Jobs' state filters after them (they belong *to* Jobs rather than
being peers of it, and nothing shifts position when you switch). `a` reaches
Agents in one keystroke; the Agents tab carries a `2!` badge so a blocked agent
is visible from the Jobs view.

```
  Jobs   Agents 2!                                 │  Agents 2   Jobs 3   │ All 3  Running …
  AGENT            STATE       FOR   KIND   BRANCH
! lint             ERROR       50s   codex  chore/lint
   "exit 1: 3 lint failures in internal/tui"
! review-api       WAITING     8m    claude feature/api
   "Should this endpoint require authentication?"
  default          QUIET       22m
   no output since 22m ago
  fix-tests        ⠋ WORKING   2m    claude feature/fix
   last output 3s ago
```

Each agent gets **two lines**: what it is, and what it asked. That second line
is the whole reason the view exists — in the 26-column sidebar a name, a state
and an age already fill the row, so the question you actually have to answer
had nowhere to go. `c` compacts to one line each for triaging many at once.
`⏎` attaches, `!` acknowledges, `e` edits the hosting session.

A **QUIET** agent is one claiming to work while printing nothing for five
minutes. That is the single failure an agent cannot report — being wedged is not
a state anything gets to announce — so it is derived from the output stream and
phrased as a doubt rather than a failure.

The Jobs view has a **session tab row at the top**, with **All sessions** for
the combined queue. Click a session or use `[` / `]` to select it; the job
list and status counts are scoped to that session. Below it are **All**,
**Running**, **Queued**, **Done**, and **Failed**
(`←` / `→` also roll these, from the table only — in the sidebar they stay fold
keys). The active tab shows its job count, and **Failed** keeps its count
visible from any view.

```
╭────────────────────────╮ ╭──────────────────────────────────────────────────────────╮
│  Spaces 2   Agents 2!  │ │  All 3   Running  Queued  Done  Failed 1                 │
│ ▾ DEFAULT           1R │ │   ID   GROUP ▲  SESSION  STATE    TIME  WORK             │
│›▾ agent-1      WAIT 14m│ │▌   0   default  build    DONE     40s   make all         │
│     claude     WAIT 14m│ │    1   default  build    RUNNING  4s    tests  go test   │
│ ▾ build             1R │ │    2   default  build    EXIT 2   1s    go vet ./...     │
│    #0 compile          │ ╰──────────────────────────────────────────────────────────╯
│                        │ ╭──────────────────────────────────────────────────────────╮
│                        │ │  job details ────────────────────────────────────────────│
│                        │ │  Command: make all                                       │
│                        │ │  State: finished                                         │
╰────────────────────────╯ ╰──────────────────────────────────────────────────────────╯
 j/k  move   tab  agents   l/h  fold   ⏎  open   m  menu   e  edit   n  new   d  delete
 MANAGE  VIEW All  SESS 3  MSG Ready          CPU  4% ▂░░░░░░░  MEM 2.6G/15.0G ▂░░░░░░░
```

`Tab` from there switches the sidebar to its agent list:

```
╭────────────────────────╮
│  Spaces   Agents 4     │
│▌ agent-2 · p2   ERR 50s│
│  agent-1       WAIT 14m│
│  agent-2 · p1      ⠹ 3m│
│  default               │
╰────────────────────────╯
```

In **Spaces** the same agents appear under their own sessions instead, marked
their kind — an agent needing you above the session's jobs, a quiet
one below them:

```
╭────────────────────────╮
│  Spaces 2   Agents 4!  │
│ ▾ WORK          1R 1Q  │
│›▾ agent-1      WAIT 14m│
│     claude     WAIT 14m│   ← waiting: above the jobs
│ ▾ agent-2         ⠹ 3m │
│     codex p2    ERR 1m │   ← errored: above the jobs
│    #4 go vet           │
│     codex p1      ⠹ 3m │   ← working: below them
╰────────────────────────╯
```

An agent row is one pane, so two agents in one session stay two rows (the pane
id is appended only when a session runs more than one). The right-hand column
is how long it has been in that state; `Enter` attaches **directly to that
agent's pane** — not just its session — which is also how you acknowledge a
`waiting` pane. If the pane has closed since the last poll, you land on the
session's first pane instead of nowhere. What the agent last *said* goes in the
Details pane rather than the row — at 26 columns a clipped note reads as part
of the name.

Color is spent on urgency alone. The tree's hierarchy comes from weight
instead — group bold, session bright, agent faint, job in its state color — so
an agent reads as a detail of its session until it has something to say, at
which point its state color takes the row. Giving "is an agent" a hue of its own
left nothing louder for "this agent is stuck".

Note that the status bar's `N sessions need you` counts sessions while the
**Agents** badge counts panes: a session with two blocked agents is one session
and two agents.

The table budgets its columns by usefulness rather than a fixed layout, so it
stays readable in a narrow terminal:

- **ID**, **STATE**, **TIME** and **WORK** always show. **WORK** is what the job
  *is* — a named job leads with its name and trails the command in a dim tone
  when both fit; an unnamed job shows the command once.
- **SESSION** and **GROUP** are added when there is room for them, widest
  terminal first, and dropped entirely rather than squeezed when there isn't.
  Both are always in the Details pane.
- **TIMEOUT** (wall-clock limit, `-` = none) appears only when some job in view
  actually has one, since it is opt-in per job.

At 80 columns you get `ID STATE TIME WORK`; around 100, SESSION joins; past
about 110, GROUP does too.

**The hint bar under the panes is clickable.** It already lists exactly the keys
that apply right now, so each entry is a button: clicking one replays the key it
advertises, through the same handler the keyboard uses. In a session that
includes `^B d detach`, which was otherwise reachable only by knowing the
chord. Entries naming two keys (`j/k`, `gg/G`) are not buttons — there is no
single thing they could do.

**Double-click opens** what a single click selects, in the sidebar and the job
table alike. A single click still only moves the cursor, so a stray one cannot
launch you into a session.

Press `Enter` on a job to open its output in the pager. Empty sessions have a
placeholder row; `Enter` opens their shells. Use the session picker (`S`) to
open any session, and `Ctrl+B d` to return to the management screen.

Under the panes, a hint bar lists the keys that apply right now (it turns into
the prompt while filtering or typing a `:` command, and into the question while
confirming). The status bar below it shows the current mode — `MANAGE` (the
table), `INSERT` (a shell pane), or `COMMAND` (a command prompt) — the active
view, the session count, the last message, and CPU, memory, and system load.
Inside a session, it also shows the focused pane's Git branch.

Bare `ru` and `ru -S` both open this screen; `ru -j` opens it in **jobs-only**
mode, and `ru -l` still just prints the job list to stdout. `q` quits from this
screen (daemon-hosted shells keep running); `Esc` returns to the open session.

Only one interactive `ru` runs per daemon (like `tmux attach -d`): starting a
second one takes over — the earlier instance exits with a "detached" notice
instead of both mirroring the same shells and fighting over their sizes.
Running `ru` from *inside* a runic pane doesn't take over; it surfaces the
already-running TUI's management screen. Mouse clicks select job rows and switch
session tabs. In session view, click a sidebar session to attach directly.
Use Shift-drag for native terminal text selection (where supported). Press `?`
on the management screen for a key/command cheatsheet.

**Management key bindings:**

| Key | Action |
|-----|--------|
| `j`/`k`, `gg`/`G`, `Ctrl+d`/`Ctrl+u` | Move / jump / half-page |
| `Space` | Select the cursor job and move down (ranger/lf-style); selected rows stay highlighted, `Esc` clears |
| `Enter` | Open the cursor job's **output** (the pager); on an empty-session row, open that session's shell panes |
| `o` | Open the output pager (scrollable; follows running jobs); press `i` there to overlay job info |
| `S` | Open the session picker, including empty sessions |
| `Tab` / `Shift+Tab` | Roll the **focused** pane's views — the sidebar between Spaces and Agents, the table through All, Running, Queued, Done, Failed (`←` / `→` also roll the table's, from the table) |
| `,n` | Toggle the session sidebar |
| `Ctrl+w h` / `Ctrl+w l` | Focus the sidebar / the job table |
| `h` / `l` | Collapse / toggle a node in the sidebar (Spaces view; the Agents list is flat) |
| `m` / `A` | Open the sidebar action menu / toggle sidebar zoom |
| `e` | Edit the cursor row in one box — a job row: job name, its session's name/group, and its timeout (empty = none); a session row: name + group |
| `n` | Create a new session (opens the edit box; you land in it) — name, group, **agent**, and branch |
| `a` | Switch the main pane to the **Agents** view |
| `c` | In Agents: toggle one-line / two-line rows |
| `sg` / `si` / `ss` / `st` | Sort by group / id / state / time; the same chord again (or `R`) reverses |
| `/` | Filter by command/label/session (`Enter` apply, `Esc` cancel) |
| `:` | Command line (see below) |
| `?` | Show the help overlay — a scrollable page (`j`/`k`, `^d`/`^u`, `gg`/`G`), closed with `Esc`, `q` or `?`. It opens on the keys that apply right now |
| `!` | Acknowledge the agent under the cursor without attaching to it |
| `V` | Toggle visual mode; `j`/`k` extend the selection |
| `x` / `u` / `r` | Kill / make urgent / rerun the selected job(s) — the Space-selected set, the visual range, or the cursor row |
| `d` | Remove the selected job(s); finished jobs delete immediately, others confirm `y`/`n` |
| `D` / `C` | Delete all finished jobs (no confirm / confirm `y`/`n`) |
| `q` | Quit (daemon-hosted shells keep running) — from the sidebar too; the action menu still dismisses on `q` |
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

`Tab` / `Shift+Tab` always move between the box's fields, as do `↑` / `↓`. On
the fields that offer a choice — **group** and **agent** — `←` / `→` pick from
it, with the options listed in the box as you go.

`e` (edit) and `n` (new) open a small modal to set a session's **name** and
**group**; submitting renames/moves or creates it. While the group field is
focused, the existing groups are listed in the box with the current match
highlighted — `Tab` steps through them, or type a new name to create a group
on save. On a **group header** row, `e` renames the group and `d` deletes it
(empty groups only, with confirmation).

#### Session tabs

Every open session is a tab along the top border of the terminal view, so the
sessions you have running are visible from inside any one of them:

```
╭ ! agent-1   ⠹ agent-2   broken   default ───────────────────────────╮
│ $                                                                   │
```

The active session is a filled chip; a session that has reported something wears
a one-column mark — `!` for anything still wanting you, the live spinner while
it works — and takes that state's color across the whole tab, so it reads at a
glance. A tab has room for one column, which is why it gets a mark where a tree
row gets the word. That is the point of the strip — an agent that starts **waiting**
on you shows up on its tab while you are working in a different session, which
previously meant detaching to the management screen to notice. A `working` agent
shows the live `⠹` spinner; an error you have already acknowledged shows no mark
at all, since it is no longer news.

Roll between tabs with `Ctrl+B n` / `Ctrl+B p`, or jump straight to one with
`Ctrl+B 1`…`9`. Tabs are ordered by name, so a digit keeps pointing at the same
session across a detach and reattach when the session catalog is unchanged.
Tabs include every session, including ones not yet opened by this client.
Long tab rows keep the selected session visible and indicate hidden neighbours
with arrows; the same keyboard shortcuts reach those sessions.

The **Spaces / Agents sidebar stays visible beside the terminal** at widths of
60 columns or more. Spaces groups sessions without cluttering the sidebar with
job rows; Agents lists reported panes by urgency. The selected session's state
and note appear at the bottom, and `›` marks the active session.

Click a session or agent to attach, or use `Ctrl+B s` to focus the sidebar:
`j`/`k` select, `Enter` opens, `n` creates, `e` renames or moves to a group,
`d` requests deletion, and `Tab` switches Spaces / Agents. `Esc` returns focus
to the terminal. `Ctrl+B b` hides or restores the sidebar. On narrow terminals,
`Ctrl+B s` opens the management sidebar. Mouse reporting is enabled for direct
navigation; use Shift-drag for native selection where your terminal supports it.

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
| `Ctrl+B` `s` | — | Focus session sidebar |
| `Ctrl+B` `b` | — | Hide / show session sidebar |
| `Ctrl+B` `n` / `p` | — | Switch to the next / previous **session tab** |
| `Ctrl+B` `1`…`9` | — | Switch to the *n*th session tab |
| `Ctrl+B` `o` | `o` / `next` | Cycle focus to the next pane (*within* the session) |
| `Ctrl+B` arrows / `h` `k` `l` | — | Move focus to the adjacent pane |
| `Ctrl+W` then `h`/`j`/`k`/`l` / `w` | — | Move / cycle focus between panes |
| `Ctrl+B` `x` / `Ctrl+W` `q` | `x` / `close` | Close the focused pane (last pane is kept) |
| `Ctrl+B` `z` | `z` / `zoom` | Zoom the focused pane to the whole region; again restores the layout. Splitting, closing a pane or switching session releases it |
| `Ctrl+B` `?` | — | The help page |
| `Ctrl+B` `d` | `detach` | Detach back to the management screen (shells persist) |
| `Ctrl+B` `j` | `jobs` / `j` | Jump to the management screen |
| `Ctrl+B` `c` | — | Open command mode |
| `Ctrl+B` `q` | `q` / `quit` | Quit |

**Pressing `Ctrl+B` lists what it can do.** The footer becomes a menu of the
chords that apply right now — and those entries are clickable, like the rest of
the hint bar. A chord you have to know already is not something anyone
discovers, and the help page is the wrong place to learn it: you want it at the
moment your fingers are on the prefix.

The prefix stays active until the next key. Press `Esc` after `Ctrl+B`
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
| `theme` | `RUNIC_THEME` | TUI palette: `auto` asks the terminal for its background (works inside tmux), or force `light` / `dark` | `auto` |
| `agents` | `RUNIC_AGENTS` | Agent kinds the New-session box offers: `kind=command`, comma-separated, merged over the built-ins | `claude`, `codex`, `gemini`, `aider` |

`RUNIC_SESSION` (environment only) sets the default session when `-g` is
omitted; the TUI sets it inside session shells. Legacy `TS_*` names are still
accepted as fallbacks for compatibility.

## Managing coding agents

Run coding agents in session panes to keep them running when you close the
interface. Attention states show which agents need input; worktree sessions give
each agent a separate checkout.

### Start an agent with the session

A session can name the agent it hosts, and runic starts it for you:

```bash
ru session create agent-1 --agent claude                 # start claude in it
ru session create agent-2 --agent codex --worktree       # …on its own branch
```

In the TUI, the **New session** box (`n`) has an `agent` field. `Tab` cycles the
configured kinds — `none`, `claude`, `codex`, … — and the choices are listed in
the box as you go, so it is a visible choice rather than a blind rotation.
Picking one prefills `branch` with the session name, since an agent usually
wants its own worktree; clear it for an agent that should work in the main
checkout, and a branch you typed yourself is never overwritten.

```
╭─ New session ──────────────────────────────╮
│                                            │
│▌ name     agent-1                          │
│  group    work                             │
│  agent    claude                           │
│  branch   agent-1                          │
│                                            │
│   none  ▌claude▐  codex  gemini  aider     │
│                                            │
│  agent: started in the session's first     │
│  pane, which keeps a shell underneath it   │
│  tab/shift+tab: agent · ↑/↓: switch · ⏎    │
╰────────────────────────────────────────────╯
```

The agent runs **in** the session's first pane rather than as it: the pane is a
normal shell that launches the agent, so the prompt marker keeps working, the
pane survives the agent exiting, and splitting the session gives you a shell
next to the agent rather than a second copy of it. The pane reports `working`
from the moment it starts, so a new agent session shows up in the **Agents** tab
immediately instead of waiting for its first `ru mark`.

Which command each kind runs is configurable — the built-in guesses assume the
tool is on `PATH` under its own name:

```bash
# Override a built-in, add your own wrapper, or hide one you don't have
ru config set agents 'claude=claude --permission-mode plan, mine=~/bin/my-agent, aider='
```

Entries merge over the built-ins (`claude`, `codex`, `gemini`, `aider`), so
naming one tool does not hide the rest; `kind=` with no command removes a kind.
An unknown kind fails the create rather than quietly giving you a plain shell.

### Report attention with `ru mark`

Every pane carries a self-reported **attention state**, so the session list reads
like an inbox instead of a wall of identical shells:

| State | Meaning | In the tree |
|-------|---------|-------------|
| `waiting` | blocked on you | `WAIT` amber, floats to the top, with how long |
| `error` | stopped on a failure | `ERR` red, floats to the top, with how long |
| `waiting`/`error` (seen) | still true, acknowledged | same word, stated quietly, stays put |
| `working` | busy, needs nothing | an animated `⠋` spinner in green, with how long |
| `done` / `idle` | nothing to do | plain |

States are **words, not symbols**. `● ◐ × ◆ ⊕` was nine shape families competing
in twenty-six columns, which reads as a 1990s file manager however carefully
each glyph is chosen; a short tag needs no legend and survives a terminal with
no color. A working agent gets the spinner instead, because motion is the one
thing a static row cannot fake — and it only animates while something is
actually working, so an idle dashboard wakes on its poll and nothing else.

The sidebar shows how long a session has needed attention, such as `14m`.
Select a session to see its branch, worktree, latest note, and job counts in
the detail pane.

The sidebar's **Agents** tab is the per-pane view of the same states: one row
per live pane rather than one per session, sorted by urgency across every
session, so two agents in one session are two rows. Use **Spaces** to see how
work is organized and **Agents** to see who needs you.

A process inside a pane reports its own state. Panes get `RUNIC_SESSION` and
`RUNIC_PANE` in their environment, so no configuration is needed:

```bash
ru mark working                      # from inside a pane
ru mark waiting "which schema?"      # the note shows in the tree
ru mark error "tests failed"
ru mark done "Fixed navigation. Tests passed."
ru mark idle --session agent-1 --pane p2   # or target one explicitly
```

The main **Agents** view shows a completed agent's note as a `Result:` preview
in its default two-line rows. Press Enter to open the session for the full
response; compact rows hide the preview. Starting the next task clears the old
note when the agent reports `working`.

Sessions roll up to their most urgent pane, `N sessions need you` appears on the
status bar, and the sessions needing you sort to the top of their group. The
sidebar's **Agents** tab counts and lists the individual panes instead, so the
two numbers differ when one session holds several agents. The state is
**self-reported on purpose**: a full-screen agent lives in the alt-screen, where
guessing from output is unreliable.

Two things adjust it automatically, and both are facts rather than guesses:

- A shell prompt clears a stale `working` — whatever was running has exited.
- Attaching to a pane acknowledges its alert. What that means depends on the
  state, because **acknowledging is not fixing**:
  Acknowledging is never resolving: the state and the note stay on the row, and
  the pane only stops counting toward `N sessions need you` and stops sorting to
  the top. Only a fresh report clears it — which, with the agent's hooks wired,
  happens the moment you actually answer.

  This applies to `waiting` as much as to `error`. Attaching used to clear a
  waiting pane outright, on the reasoning that it was blocked on you and now you
  are there. That was wrong in the case that matters: read the question, realize
  you need to go and find the answer, leave — and the row reported `idle` with
  the question discarded while the agent sat there still blocked.

A new report always resets the acknowledgment, so an agent that errors *again*
after you looked at the last failure gets your attention back.

You can also acknowledge **without attaching**: `!` on a sidebar row, or
`ru ack` from a script. Attaching is a heavy way to say "noted" — it takes over
your screen — and on a waiting agent it is easy to mistake for having answered.

### The alarm

When a pane *starts* needing you, runic says so rather than waiting to be
noticed:

- the **terminal bell** rings once, for when you are looking at something else;
- a **status-bar alert** names the agent and quotes what it asked —
  `! agent-1 · p2: which schema?` — and stays up until that pane is attended to
  or acknowledged.

Both fire on the transition, not on every poll, so an agent you have already
been told about stays quiet; an acknowledged error stops alarming entirely. Two
agents blocking at once are two alerts, and the one kept waiting longest is the
one named. If your terminal's bell is off, the status-bar alert still carries
it, and `on_attention` (below) covers being told while the TUI isn't open.

#### Self-reporting is wired for you

A session you create with `--agent claude` is launched **already instrumented**:
runic passes the hooks to Claude Code inline (`claude --settings '<json>'`), so
nothing is written into your repository and your own settings are untouched.
This matters more than it sounds — an agent that never calls `ru mark` leaves
every attention state empty, and the sort, the chips, the tab glyphs and the
alarm all sit idle with nothing to report. It used to be a manual step, and a
session that skipped it looked identical to a healthy one.

Instrumentation only ever touches a command runic recognizes: the first word has
to be `claude`. Point the kind at your own wrapper and the command is used
verbatim — appending a flag to an arbitrary program is a guess, and a launch
that fails outright would be worse than one that does not self-report. The
daemon log says so when it skips. An explicit `--settings` of your own also
wins.

For an agent **you** started — not launched by runic — print the snippet and
wire it yourself:

```bash
ru agent hooks claude    # prints the .claude/settings.json block
```

Which is the same thing, by hand, in `.claude/settings.json`:

```json
{
  "hooks": {
    "Notification": [
      {"hooks": [{"type": "command", "command": "ru mark waiting \"$(jq -r .message)\" 2>/dev/null || true"}]}
    ],
    "Stop": [
      {"hooks": [{"type": "command", "command": "ru mark done --claude-stop 2>/dev/null || true"}]}
    ],
    "UserPromptSubmit": [
      {"hooks": [{"type": "command", "command": "ru mark working 2>/dev/null || true"}]}
    ]
  }
}
```

The `|| true` keeps the hook harmless when the agent is run outside a runic
pane, and `ru agent hooks` prints exactly this block.

The Stop hook reads Claude's `last_assistant_message` directly from JSON on
stdin, without `jq`, and stores up to 2,000 characters as the completion note.
This is an excerpt of the actual response, not a generated summary. Missing or
invalid payloads still mark the agent done, without a preview. See the
[Claude Stop hook reference](https://code.claude.com/docs/en/hooks#stop-input).
Automatic previews apply to Claude sessions launched with the updated hooks;
existing sessions need the updated Stop hook and a restart of Claude. Other
agents can supply the same preview with `ru mark done "summary"`.
Runic's daemon installs these hooks at agent launch, so a daemon started before
this update must also be restarted after its current work finishes. Installing
the binary or restarting just the TUI does not update a running daemon's hooks.

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
ru session create agent-4 --worktree --agent claude  # isolated, agent started
```

`--agent` and `--worktree` are independent: an agent in the main checkout is a
legitimate thing to want, so neither implies the other.

In the TUI, the **New session** box (`n`) has a `branch` field: leave it blank
for an ordinary session, or name a branch to isolate it. The branch appears in the
pane status bar, the Details panel, and the tree's action menu (`m`).

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
- Agent transition history (`ru wait --agent`, "how often did this agent block?")
- Slots as an agent concurrency/cost governor (cap N agents, queue the rest)
- Auto-enqueue a follow-up job when an agent finishes (run tests → open a PR)

## License

MIT
