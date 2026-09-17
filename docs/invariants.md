# Invariants

Behavioral documentation tells you *what* the code does; this file documents
the properties the code must *never* violate, no matter what inputs,
failures, or race conditions arrive. These invariants are the load-bearing
walls behind every bug fix so far — they are written down so that future
refactoring can be checked against them, and so a regression is recognizable
as a broken invariant rather than a mysterious behavior change.

When you change the daemon, ask: *which invariant does this touch, and
what test proves it still holds?* If you cannot point at the test, the
invariant is now load-bearing again — add the test.

---

## Inhibitor invariant

> If the daemon is alive and logind is available, eventually exactly one
> valid inhibitor is held.

- **Eventually**: the inhibitor is a retrying state machine
  (`internal/daemon/inhibitor.go`), not a one-shot acquisition. A missing
  or unavailable system bus at startup is a *failure state*
  (`StateAcquiring` / `StateFailed`), never a startup error — `NewInhibitor`
  never fails. While the lock is lost, logind handles the lid itself; that
  double-handler window is the documented exception, and the retry loop
  (every 5 s) exists to shrink it.
- **Exactly one**: the lock is represented by the fd returned from
  `Inhibit()`. `acquireOnce` closes any previously held fd *before*
  installing a new one, and `releaseFd` is idempotent, so no code path can
  hold two live fds at once. Two live fds would mean systemd attributes the
  lid to two owners simultaneously.
- **Valid**: a held fd that no longer corresponds to a live logind is not
  held — it is *lost*. The `lost()` channel (logind releasing its bus
  name) and the 250 ms liveness poll (a silently dropped D-Bus
  connection) both force a drop-and-reacquire, so the only steady state is
  `StateAcquired` with a live connection.

## Display invariant

> Only outputs recorded in `disabledByUs` may be restored.

- **What it protects**: `Handler.disabledByUs`
  (`internal/daemon/handler.go`) records exactly the outputs *sway-power*
  disabled, and `HandleLidOpen` restores exactly those names via
  `action.ShowInternalDisplay`. Outputs the user disabled themselves are
  not in the map, so they are never re-enabled behind the user's back.
- **What feeds it**: only the return value of `Action.Execute` enters the
  map — outputs actually disabled, never intended-to-be-disabled. A failed
  `Execute` (e.g. swaymsg down) claims nothing.
- **What leaves it**: an entry is removed only after its restore *succeeds*.
  A failed restore keeps the entry so the next lid-open retries; this is
  how a transient swaymsg failure self-heals instead of silently leaving a
  dead panel forever.

## Action invariant

> Changing the configured action does not retroactively change the display
> ownership acquired by a previous close event.

- Display ownership (`disabledByUs`) is tracked **independently of the
  current action**. This is why `Action.OnOpen()` is a no-op by design:
  restore is driven by the ownership map in `HandleLidOpen`, not by the
  action that happens to be configured.
- The motivating scenario: lid closed with `nothing` (panel disabled,
  ownership recorded), user switches to `lock` or `sleep` while the lid is
  closed, lid opens — the panel is still restored, because ownership was
  acquired by the *previous* close event and survives the action swap.
- Corollary: `SetAction` only swaps `Handler.current`; it never touches
  `disabledByUs`. The two fields have independent lifecycles on
  purpose.

## Monitor invariant

> Each logical lid transition is delivered at most once.

- The monitor has two sources of truth (evdev event stream + 500 ms state
  file poll, `internal/daemon/monitor.go`) feeding a single change
  detector in `run()`. Both can observe the same physical transition —
  evdev reports it immediately, the next poll reports it again — so the
  dedup (`if have && s == last`) is not an optimization, it is the
  invariant: the callback fires at most once per actual state change.
- **At most once, not exactly once**: a missed transition is caught by the
  polling backstop (the next 500 ms read corrects it), so the monitor is
  eventually consistent per physical state. What is *never* allowed is
  the callback firing twice for the same transition — that would double
  execute the close action or double-restore displays.
- The initial-state callback (from the state file on startup) is not a
  transition; downstream must treat it as "current state", not "lid
  just moved".

## Preference invariant

> A malformed preference file never replaces the last known-good action.

- The daemon path loads via `preferences.LoadFromPath`, which distinguishes
  three outcomes that must not be conflated:
  - **missing file** → not an error; means "no preferences yet", defaults
    apply. (A first run or a fresh system.)
  - **corrupt / unreadable / unparseable file** → an error. The preferences
    watcher logs it and **keeps its current action**. The last known-good
    action survives.
  - **parseable but invalid action string** → also kept: the watcher
    re-validates the loaded action (`newAction.Validate()`) before
    `SetAction`, and `Handler.SetAction` re-validates as a second gate.
- The GUI path (`preferences.Load`) deliberately swallows errors and
  degrades to defaults — it has no last-known-good state to protect. Do not
  unify these two: the invariant above exists precisely because the daemon
  cannot afford `Load`'s semantics.
- Writes are atomic (tmp file + rename, `preferences.Save`), so a writer
  crash cannot produce a torn file; but a *user-edited* or *manually
  corrupted* file is exactly the failure mode this invariant is about —
  the poller must tolerate it every second, not just at startup.
