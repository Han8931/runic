# TODO

## Adapt runic to manage coding agents

runic already has the primitives: daemon-hosted persistent PTY panes, sessions/
groups, a queue with states + slots + timeouts/retries, job dependencies, a
git-branch-aware status bar, and OSC 133 prompt-marker parsing (`atPrompt`). The
gap from "shell session manager" to "agent manager" is mostly **attention,
isolation, and lifecycle**.

### Priority 1 — attention column + `ru mark` hook — DONE
- [x] Per-pane state `working` / `waiting` / `idle` / `errored` on `TerminalPTY`
      (`internal/server/attention.go`), rolled up to the session and surfaced in
      the TUI tree: colored row + glyph, "needs you" sorted to the top of its
      group, and an `N need you` chip on the status bar.
- [x] `ru mark <working|waiting|done|error|clear> [note]` over `MsgMark`. Panes
      get `RUNIC_SESSION` **and** `RUNIC_PANE`, so a process reports its own
      state with no configuration.
- [x] Claude Code `Stop` / `Notification` / `UserPromptSubmit` hook snippet in
      the README, so the agent self-reports instead of runic guessing.
- [x] Automatic adjustments that are facts, not guesses: a shell prompt clears a
      stale `working`; attaching to a pane clears `waiting`/`error`.

### Priority 2 — notifications — DONE
- [x] `on_attention` config key, fired on every transition with
      `RUNIC_SESSION`, `RUNIC_PANE`, `RUNIC_ATTENTION`, `RUNIC_ATTENTION_FROM`,
      and `RUNIC_NOTE` so the hook can filter.
- [ ] Not done: an in-TUI terminal bell. The visual signals (colored row, chip)
      cover the attached case; a bell needs a way to emit BEL exactly once
      without racing bubbletea's renderer.

### Priority 3 — worktree-per-agent isolation — DONE
- [x] `ru session create <n> --worktree [--branch <b>] [--repo <p>]`, and a
      `branch` field in the TUI's New-session box. Panes start in the worktree;
      the tree marks the session `⑂`.
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

### Priority 4 — reuse existing mechanisms
- [ ] Slots as a concurrency/cost governor: cap N agents running at once
      (API rate limits / cost), queue the rest — same mechanism as job slots.
- [ ] Chaining via existing job dependencies: on agent finish, auto-enqueue a
      follow-up (run tests → if green, `gh pr create`). Wants worktree-aware job
      working directories first.

### Detection notes
- Full-screen agents live in the alt-screen, so output-pattern detection is
  unreliable — `ru mark` is the primary signal, by design. The only output the
  daemon reads for state is the OSC 133 prompt marker, which means "the shell is
  back at a prompt" and nothing more.
