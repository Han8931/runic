# TODO

## Adapt runic to manage coding agents

runic already has the primitives: daemon-hosted persistent PTY panes, sessions/
groups, a queue with states + slots + timeouts/retries, job dependencies, a
git-branch-aware status bar, and OSC 133 prompt-marker parsing (`atPrompt`). The
gap from "shell session manager" to "agent manager" is mostly **attention,
isolation, and lifecycle**.

Attention, isolation, lifecycle correctness and the Agents view (P1–P4) are
done. What remains is that an agent is still not a *thing* in the model, only a
state on a pane — see Priority 5, which P6 depends on.

### Priority 1 — attention column + `ru mark` hook — DONE
- [x] Per-pane state `working` / `waiting` / `idle` / `errored` on `TerminalPTY`
      (`internal/server/attention.go`), rolled up to the session and surfaced in
      the TUI tree: colored row + glyph, "needs you" sorted to the top of its
      group, and an `N need you` chip on the status bar.
- [x] `ru mark <working|waiting|done|error|clear> [note]` over `MsgMark`. Panes
      get `RUNIC_SESSION` **and** `RUNIC_PANE`, so a process reports its own
      state with no configuration.
- [x] Claude Code `Stop` / `Notification` / `UserPromptSubmit` hooks. Originally
      a README snippet to paste; now **wired automatically** for a session runic
      starts (`internal/agent`), passed inline via `claude --settings '<json>'`
      so nothing is written into the user's repo. The manual step was the real
      bug: an agent that never calls `ru mark` leaves every state empty, so the
      sort, chips, tab glyphs and alarm all sat idle with nothing to report —
      indistinguishable from the feature not existing. `ru agent hooks` prints
      the block for an agent runic did not launch.
      Instrumentation is narrow on purpose: only a command whose first word is
      `claude`, never a user's wrapper script, since appending a flag to an
      arbitrary program is a guess and a failed launch is worse than a quiet one.
- [x] Automatic adjustments that are facts, not guesses: a shell prompt clears a
      stale `working`; attaching to a pane clears `waiting`/`error`.

### Priority 2 — notifications — DONE
- [x] `on_attention` config key, fired on every transition with
      `RUNIC_SESSION`, `RUNIC_PANE`, `RUNIC_ATTENTION`, `RUNIC_ATTENTION_FROM`,
      and `RUNIC_NOTE` so the hook can filter.
- [x] An in-TUI alarm (`internal/tui/alarm.go`): the bell, plus a status-bar
      alert naming the agent and quoting what it asked. Both fire on the
      transition into needing you — keyed per *pane*, so two agents blocking at
      once are two alerts and a pane already announced stays quiet.
- [x] The renderer race is solved by emitting the BEL **inside the frame**:
      `View` prefixes `"\a"` and `Update` clears the flag, so the bell rides the
      renderer's own single write and cannot land inside an escape sequence.
      BEL costs no display columns, which a test pins by checking every line's
      width with the bell set.

### Priority 3 — worktree-per-agent isolation — DONE
- [x] `ru session create <n> --worktree [--branch <b>] [--repo <p>]`, and a
      `branch` field in the TUI's New-session box. Panes start in the worktree;
      the tree marks the session `⊕`.
- [x] Auto-clean on session delete (and on `:reset`). The directory always lives
      under `~/.local/state/runic/worktrees/`, never a caller-named path, so the
      daemon only ever removes what it created. The branch is kept — and so is
      the whole worktree when it still holds uncommitted or untracked work:
      the delete is refused and names the files, `--discard` (TUI: a second
      confirmation) is the only way to lose them. `:reset` always discards.
- [ ] Not done: attaching a worktree to an *existing* session, or running the
      session's queued jobs in the worktree (they still use their submit-time
      cwd — see Priority 4, which needs it).

### Lifecycle correctness — DONE
- [x] A kill is not a failure: `ru -k` (and `:kill`, `x`, `kill -a`) records the
      cancellation before signalling, so a job with retries left is resolved
      rather than restarted, and it reports `canceled` instead of a signal death.
- [x] Renaming a session carries its live panes with it rather than stranding
      them under the old name, so attention roll-up, layout and delete all keep
      pointing at one session. Panes are identified by their own (permanent)
      name, which is how `ru mark` from a shell that still has the old
      `RUNIC_SESSION` in its environment still lands.
- [x] Queue snapshots are serialized end to end (encode + write under one lock),
      so two concurrent saves can no longer land out of order and let an older
      snapshot overwrite a newer one.

### Priority 4 — the Agents view — DONE
- [x] The Sessions sidebar carries view tabs, laid out like the Jobs pane's:
      **Spaces** (the group → session → job tree; a group *is* a workspace) and
      **Agents** (one row per live pane that has reported a state, flat, sorted
      by urgency across every session). `Tab`/`Shift+Tab` roll the *focused*
      pane's views, so one gesture always means "next view"; `←`/`→` stay
      table-only. Clicking a tab switches it, and clicking a sidebar row now
      selects it (it previously did nothing).
- [x] Agent rows sort errors → waiting → working → idle, oldest-first within a
      state, with session/pane breaking ties so the list doesn't reshuffle under
      the cursor between polls. Two agents in one session stay two rows.
- [x] The TUI polls `MsgTerminalListAll` alongside the tree, tolerating its
      failure: a dropped call empties the Agents tab instead of blanking the
      screen. Sessions only carry the rolled-up attention, so per-pane state has
      to come from its own query.
- [x] Agents and spaces get their own Details panels. The session panel's
      worktree/job fields are meaningless for a single pane, and an agent's note
      lives there rather than in the row — at 26 columns "tests failed" clips to
      "tests" and reads as part of the name.
- [x] `Enter` on an agent row lands on *that agent's pane*, via
      `openSessionPane` → `focusPaneNamed`. A pane that closed between the poll
      and the keypress falls back to the session's first leaf rather than
      stranding focus. (No resize is needed on the focus change: pane geometry
      comes from the layout, which `activateSession` has already sized.)

### Priority 4b — responsive job columns — DONE
- [x] The table budgets columns by usefulness instead of a fixed layout. ID,
      STATE, TIME and WORK always survive; SESSION and GROUP are added only when
      they fit with WORK's floor intact, and are dropped entirely rather than
      squeezed. TIMEOUT appears only when a job in view has one.
- [x] NAME and COMMAND merged into one WORK column. They held *the same text*
      for unnamed jobs, which is what starved the table: at 80 columns the old
      budget gave COMMAND one character and pushed WORK off the pane entirely —
      the table rendered without ever saying what any job was. At 100 it showed
      `go tes` / `go test`.
- [x] Hidden columns drop their separator too, so nothing holds empty space;
      `TestJobRowFitsItsBudget` pins the row to exactly its allotted width and
      `TestJobsHeaderMatchesVisibleColumns` keeps the header honest.

### Priority 4c — acknowledged vs resolved alerts — DONE
- [x] Attaching no longer erases an error. `seen()` splits the two cases:
      `waiting` is *resolved* by attaching (it was blocked on you, you are now
      there) and still clears to idle; `error` is only *acknowledged* — state
      and note survive, and it merely stops counting toward "needs you" and
      stops floating to the top. Only `ru mark done` or a fresh mark clears it.
      Previously glancing at a failure made the dashboard look healthier with
      nothing fixed.
- [x] `protocol.Unresolved(attention, seen)` is the single predicate for "still
      wants your attention"; `Attention.NeedsYou()` remains a property of the
      state alone. Every consumer — the status-bar count, the tree sort, the
      sidebar chip, the Agents badge and its sort rank — goes through it.
- [x] Any new mark resets the acknowledgment, so an agent that errors again
      after you looked at the last failure gets your attention back.
- [x] The roll-up prefers an unresolved pane over an acknowledged one whatever
      their raw urgency, so a session reports what is outstanding rather than
      the loudest thing already dealt with.
- [x] Protocol 10 → 11: `Seen` on `TerminalInfo`, `AttentionSeen` on
      `SessionInfo`.
- [x] Acknowledging without attaching: `!` on a sidebar row, `ru ack` from a
      script (see Priority 4f, which also stopped attaching from *resolving* a
      waiting pane).
- [ ] Not done: "unacknowledge". `ru mark error` again is the workaround.

### Priority 4d — session tabs in the terminal view — DONE
- [x] Every session in the daemon's catalog is a tab on the top border of the
      terminal view, replacing the single-session title — including ones this
      client has not opened yet, which `activateSession` builds on demand. The
      active one is a filled chip; the others carry their attention glyph in its
      own color, which is the actual point: an agent that starts *waiting* is
      visible while you work in a different session. Previously noticing meant
      detaching to the management screen.
- [x] `Ctrl+B n`/`p` roll the tabs, `Ctrl+B 1`–`9` jump. Ordered by name, so a
      digit keeps pointing at the same session across a detach/reattach — which
      creation order would not. `Ctrl+B o` still cycles panes *within* a session.
- [x] A `working` agent shows `◐`; an acknowledged error shows no glyph, since
      it is not news (see Priority 4c). The strip falls back to the plain titled
      border when it cannot fit.
- [x] The strip is composed as a styled line rather than through the border
      grid, which renders cell-by-cell in one style and would flatten the tabs.
      `View` installs it over the region's top line, so the box is drawn
      untitled underneath.
- [x] Clicking a tab switches to it: `handleSessionMouse` hit-tests the
      `termTabSpan`s on row 0. Shift-drag still reaches the terminal's own text
      selection inside the panes, which is what the geometry was kept for.
- [x] The strip scrolls instead of being all-or-nothing. `sessionStrip` keeps
      the active tab visible, marks hidden neighbours with `‹`/`›`, and always
      fills exactly its width so the border never goes ragged — and the jobs
      view's session tabs share it rather than having a second implementation.

### Priority 4e — mouse affordances and tree legibility — DONE
- [x] **The hint bar is the toolbar.** It already lists exactly the keys that
      apply right now, so each entry is a click target and a click *replays that
      keystroke* through the same handler the keyboard uses (`clicks.go`). No
      second definition of any verb, so a hint cannot drift from its button, and
      the mouse path grew to cover everything the bar lists for free. Entries
      naming two keys (`j/k`, `gg/G`) are deliberately inert — there is no single
      thing they could do. This is also the detach button: `^B d detach` now
      leads the session footer, where it was previously reachable only by
      knowing the chord.
- [x] **Double-click opens** in the sidebar and the job table. A single click
      still only selects, keeping the guard that a stray click cannot launch you
      into a session. Pairs are tracked by *row identity*, not screen position,
      so a list that reorders under the cursor between polls cannot turn two
      clicks on different things into one activation; a third click starts a
      fresh pair rather than completing a second double.
- [x] `q` quits from the sidebar. `handleTreeKey` reported every key as handled,
      so the one key that reliably means "I am done" was swallowed in the pane
      you are most often sitting in. The action menu still dismisses on `q`:
      quitting out from under an open modal would be a nasty surprise.
- [x] **Agents are legible in the tree.** They were near-identical to session
      rows — same attention glyph, same style — so the two could not be told
      apart. Now: agents are children of their session in the Spaces tree,
      marked `◆` and named by *kind* (the session name is already on the row
      above, so repeating it spent the width twice). An agent that needs you
      sorts **above** the session's jobs; a quiet one sits below them.
- [x] Color and shape carry different facts: violet (`cViolet`, the one hue not
      already spent on state) means "this is an agent", and the attention color
      takes over when it wants something. The marker is what survives a terminal
      with no color or a reader who cannot separate violet from amber — which is
      why the distinction is not left to the palette alone.
- [x] The flat Agents tab keeps its state glyphs instead of `◆`: everything
      there is an agent, so there is nothing to tell apart, and a glyph beats a
      marker that would be identical on every row.

### Priority 4f — review follow-ups: trust and discoverability — DONE
Acting on an external review. All four of its P1s were real; two of its
*recommendations* were already in place (seen is tracked separately, and an
explicit acknowledgement exists on the wire), so what was missing was the
behaviour change and a key bound to it.

- [x] **Attaching no longer resolves a waiting pane.** It acknowledges it, like
      an error: state and note survive, only the accounting changes. The case
      that matters is reading the question, realizing you need to go and find
      the answer, and leaving — which used to report `idle` with the question
      discarded while the agent sat there still blocked. That is the same
      failure the error path was fixed for in 4c; `seen()` is now one rule
      instead of two. A fresh report is the only thing that clears a state, and
      with hooks wired that lands the moment you answer.
- [x] `!` in the sidebar and `ru ack [session] [pane]` acknowledge without
      attaching. Attaching is a heavy way to say "noted" — it takes your screen
      — and on a waiting agent it is easy to mistake for an answer.
- [x] **The status bar sheds by priority instead of by side.** It used to drop
      its whole right half the instant it did not fit, which at 80 columns took
      every navigation key with it — and, since the hint bar became clickable,
      every click target too. `fitBar` ranks segments: the mode chip, a live
      alarm and the keys that get you *out* are protected; branch, counts and
      secondary hints are shed outermost-first. `^B ?` was added so "help" is a
      real protected slot rather than an advertised chord that did not exist.
- [x] **The help overlay is a page, not a flash card.** It is ~50 rows and was
      handed to `centerBox`, which silently emitted only the first `bodyH` of
      them — past the middle was unreachable at 80x24, with nothing on screen
      admitting it. Now a viewport with `j`/`k`, `^d`/`^u`, `gg`/`G`, the wheel,
      a position readout, and dismissal only on `esc`/`q`/`?` so reaching for a
      scroll key cannot throw the page away. It opens on a contextual "Right
      now" section built from the same hints the footer renders, so it cannot
      drift from the live keymap.
- [x] **Tab means one thing in forms: next field.** It used to cycle *values* on
      group and agent while moving fields elsewhere, with Shift+Tab disagreeing
      about which. Picking a value moved to `←`/`→`, whose choices are already
      listed on screen.
- [ ] Not taken yet (P2 in the review): wrapping an agent's note instead of
      truncating it to one detail line, leading the detail panel with state and
      request ahead of metadata, and collapsing the sidebar against the table's
      minimum useful width rather than a fixed 48-column floor.

### Priority 5 — the agent as a first-class object
Partly done. A session now *has* an agent — a kind it was created to host, which
the daemon starts in its first pane — so "which agent is this?" and "is this an
agent at all?" are answerable without waiting for a `ru mark`. What is still
missing is history: `TerminalInfo` remains a current mood attached to a pane,
and both Priority 6 items are blocked on that rather than on identity.

- [x] Agent kind as a property of the session: `--agent <kind>` /
      the New-session box's `agent` field → `SessionInfo.Agent`, persisted in
      the queue snapshot and carried through renames like the worktree is.
      Kind → command lives in the `agents` config registry (built-ins merged
      with the user's), so re-pointing "claude" at a wrapper does not rewrite
      existing sessions. Protocol 11 → 12.
- [x] The launch keeps the pane a shell and runs the agent *in* it (from the
      generated rc for bash/zsh, typed in for shells runic writes no rc for), so
      the prompt marker still works, the pane outlives the agent, and a split
      gets a shell rather than a second agent. The pane is marked `working` at
      launch — a fact, not a guess — which is what makes a fresh agent session
      visible in the Agents tab immediately.
- [ ] Still missing from an `AgentInfo` the daemon owns: **transition history**
      rather than just a current value, accounting (time per state, how many
      times it blocked), and linkage to the jobs it enqueued. Provenance and
      kind now exist; these do not.
- [ ] Start with the transition history: a bounded ring buffer per pane, one new
      payload, no protocol break beyond it. It is the smallest piece that isn't
      throwaway, and every capability below reads from it. ("This agent blocked
      5× in an hour" is unanswerable today.)
- [ ] `ru wait --agent <s>[/<pane>] --for <done|idle|error>`, so a script can
      block on an agent actually finishing instead of polling. `ru -w` waits on
      *jobs* only. Easy once the history exists, awkward before.
- [x] Agent identity is no longer proxied by "has ever called `ru mark`". A
      session created with `--agent` declares its kind up front, and its first
      pane is marked at launch, so a freshly started agent is visible straight
      away. A plain `git status` shell still stays out of the Agents list,
      because it reports nothing and its session hosts no agent.

### Priority 6 — reuse existing mechanisms
- [ ] Slots as a concurrency/cost governor: cap N agents running at once
      (API rate limits / cost), queue the rest — same mechanism as job slots.
      Needs the agent record to count against.
- [ ] Chaining via existing job dependencies: on agent finish, auto-enqueue a
      follow-up (run tests → if green, `gh pr create`). Wants worktree-aware job
      working directories first, and a completion event to hang off.

### Detection notes
- Full-screen agents live in the alt-screen, so output-pattern detection is
  unreliable — `ru mark` is the primary signal, by design. The only output the
  daemon reads for state is the OSC 133 prompt marker, which means "the shell is
  back at a prompt" and nothing more.
- Deliberately **not** doing auto-detection of agent binaries (herdr detects
  15+ with zero config). It would add a second, less reliable source of truth
  for something `ru mark` already reports precisely, and the alt-screen problem
  above is why. Agent *kind* is now declared at session creation instead, which
  gets the identity without any sniffing: runic knows because it started the
  thing.

### Reference notes
- The TUI's look follows **gorae** (navy/cyan palette, bordered panes, filled
  chips, key caps) — that repo is local, so its palette is readable and was
  copied directly. **herdr** (terminal multiplexer for coding agents) informed
  the *structure* here, specifically leading with an agent list; its published
  docs carry no palette or layout spec, so nothing visual is copied from it and
  nothing should be attributed to it.
- Not pursued from herdr, in rough order of appeal: remote/SSH machines in one
  combined agent list (architectural — daemon transport, reconnect, per-host
  state); mouse-native panes inside a session (click to focus, drag borders to
  resize, right-click menu — the mouse is currently released inside panes so
  terminal text selection still works, which is a real trade-off against it).
