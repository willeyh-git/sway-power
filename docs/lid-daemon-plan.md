# Implementation plan: set-and-forget lid handling (daemon + user service)

## Problem

Today the lid monitor and the `systemd-inhibit` lock are children of the GUI
(`internal/ui/lid_buttons.go` → `lid.Monitor`). Closing the app releases the
inhibit and stops all handling, so lid behavior only exists while sway-power
is open. GNOME/KDE solve this by splitting a long-lived session daemon
(PowerDevil, gnome-power-manager) from the settings UI. We want the same
"set and forget" model: the app is only opened to *change* settings.

## Goals

- Lid handling (inhibit + state watching + actions) survives the GUI being
  closed, and starts/respawns with the session — no user intervention.
- GUI becomes a pure settings editor; it must never hold the inhibit.
- No per-distro code paths. Requirement: works unmodified on Fedora and
  Arch (both systemd + logind). No `logind.conf` is read or written.
- Settings stay in `~/.config/sway-power/preferences.json` (already persisted).

## Non-goals (explicitly out of scope for this pass)

- Expressing actions in `logind.conf` ("lock" and "nothing" are not
  expressible there; a daemon is the only way to keep all three actions).
- Improving the "nothing" + external-monitor display behavior (disable
  internal / re-enable on open). That exists today and is revisited
  separately — see **Open investigation** at the bottom.

## Target architecture

```
sway-power (GUI, on-demand)          sway-power daemon (systemd --user)
  writes preferences.json  ──────▶   holds logind Inhibit() fd
  closes, gone                           (handle-lid-switch, block)
                                      watches lid state (500 ms poll)
                                      executes lock/sleep/nothing
                                      hot-reloads preferences.json
```

Locking follows systemd's inhibitor-lock model
(https://systemd.io/INHIBITOR_LOCKS/): one
`Inhibit("handle-lid-switch", …, "block")` D-Bus call on
`org.freedesktop.login1.Manager`. The lock is tied to the returned
**file descriptor**, not to a wrapper process — it is released when the fd
is closed, and the kernel releases it automatically if the daemon dies.
(Today's `systemd-inhibit` child process in `internal/lid` is only the CLI
wrapper around this same D-Bus call; the daemon replaces it, including the
SIGTERM/verify dance.)

The unit is a `systemd --user` service, wanted by
`graphical-session.target`, with `Restart=always`. It therefore:

- starts when the graphical session starts (like PowerDevil),
- dies on logout/switch-user (no stale lock across sessions),
- is auto-restarted on crash,
- only ever needs "systemd + logind" — no distro-specific config.

## Phases

### 1. `sway-power daemon` subcommand

- `cmd/sway-power/main.go`: switch on `os.Args[1]`:
  - *(none)* → today's GUI path (unchanged).
  - `daemon` → new daemon path.
- New package `internal/daemon`:
  - Acquire the lid-switch lock **natively via D-Bus** using
    `github.com/systemd/systemd-go` (pure dbus, no cgo):
    ```go
    lm, _ := dbus.NewLoginManagerFromSystemBus()
    fd, err := lm.Inhibit("handle-lid-switch", "Sway Power",
        "Handle lid close ourselves", "block")
    ```
    Hold `fd` for process lifetime; `os.Close(fd)` on SIGTERM/SIGINT.
    No wrapper process, no registration sleep, no `--list` verification,
    no SIGTERM/kill release path — the kernel closes the fd on crash,
    which is exactly the auto-release semantics systemd documents.
    If `Inhibit()` is denied, log and continue in degraded mode
    (systemd's documented guidance: lock denial is not a hard error).
  - Load `preferences.Preferences`, validate `LidClose`
    (default `lock` when file missing — already the case).
  - Start `lid.Monitor` with a callback that runs
    `action.Execute()` on `Closed` and `action.OnOpen()` on `Open`
    (same mapping as `lid_buttons.startMonitor` today).
  - `internal/lid`: the inhibitor half (`startInhibit`, `Inhibitor`,
    `verifyInhibit`) is replaced by the fd above; `Monitor` stops taking
    the lock itself (the daemon owns it; the GUI no longer calls `Monitor`).
  - Block on SIGTERM/SIGINT; on signal, `close(fd)` and exit 0.
- This phase alone is testable headless: `sway-power daemon` can be run
  inside the sway session and must behave identically to today's GUI
  (verify with `systemd-inhibit --list` — logind lists D-Bus locks there
  too; "Sway Power" should appear).

### 2. Config hot-reload in the daemon

- Watch `preferences.json` (inotify, or 1 s poll — polling is simpler and
  there are no other file watchers in the codebase; decide by taste).
- On change: re-read via `preferences.Load()`; if `LidClose` changed and
  passes `Validate()`, rebuild the bound action and restart the monitor
  (stop old → start new). Invalid value → log, keep current.
- This removes any IPC: GUI and daemon share only the file.
- Note: restart-the-monitor is exactly what `setAction()` in the GUI
  already does, so the logic is reusable; extract it.

### 3. Unit generation + enablement (GUI side)

- On GUI startup:
  1. Ensure `~/.config/systemd/user/sway-power.service` exists; if not,
     write it (see unit file below). Always regenerate on change so
     `ExecStart` tracks the running binary (handle upgrade: unit uses the
     resolved `sway-power` path at generation time; if the binary path
     changes, GUI regenerates — cheap).
  2. `systemctl --user daemon-reload` (only if we wrote/changed the file).
  3. If `systemctl --user is-enabled sway-power` ≠ `enabled`:
     `systemctl --user enable --now sway-power`.
- Unit file:
  ```ini
  [Unit]
  Description=Sway Power lid handling
  After=graphical-session.target
  PartOf=graphical-session.target

  [Service]
  ExecStart=<resolved sway-power binary path> daemon
  Restart=always
  RestartSec=2
  Environment=WAYLAND_DISPLAY=wayland-1

  [Install]
  WantedBy=graphical-session.target
  ```
  - `WAYLAND_DISPLAY=wayland-1` is sway's default socket; the `lock`
    action (`swaylock -f`) requires it. (Refinement: read it from the GUI
    process env at generation time instead of hardcoding.)
  - `XDG_RUNTIME_DIR` is inherited from the user manager — no override
    needed.
- GUI also shows a status line ("lid handler: active/inactive") via
  `systemctl --user is-active sway-power`, refreshed occasionally.
  (Nice-to-have; can ship as its own small commit.)

### 4. Rip the monitor out of the GUI

- `internal/ui/lid_buttons.go`:
  - Delete `startMonitor`, `stopWatch`, and the
    `lid.Monitor`/`action` imports.
  - `setAction()` becomes: update buttons → `preferences.Save()` → done
    (daemon picks the change up in phase 2).
- Result: closing the app touches nothing in systemd; the service
  is the single owner of the inhibit.
- Keep `lid.Monitor`'s initial-state callback semantics: on daemon start
  with lid closed, the configured action fires once. Document that.

### 5. Tests

- `internal/daemon`: table tests for the config-file → action
  resolution (valid/missing/invalid values) — mirrors the existing
  `action_test.go` style.
- Manual checklist (Fedora + Arch):
  - close GUI → `systemctl --user status sway-power` still active,
    `systemd-inhibit --list` still shows "Sway Power"; lid close still acts.
  - change setting with app open → daemon hot-swaps (check log).
  - `kill -9` the daemon → Restart=always brings it back; lock re-acquired.
  - logout → unit gone, `systemd-inhibit --list` clean (fd auto-closed).
  - logout/login → unit starts again without reopening the app.

## Risks / edge cases

| Case | Handling |
|---|---|
| Daemon starts, `Inhibit()` D-Bus call denied (policy) | Log and continue without the lock (per systemd docs this is not an error); lid actions still run, logind may also act — degraded mode. |
| GUI opened twice | Second instance must not double-enable (it won't: `enable` is idempotent) — guard the "first run enable" path anyway. |
| Non-systemd session (no logind) | Daemon logs "inhibit unavailable", keeps watching state, actions still run — today's degraded mode, now permanent. Acceptable: out of target distros (Fedora/Arch always have both). |
| Config file hand-edited to invalid value | Daemon keeps last-good action, logs error. |
| `swaylock` missing | `lock` action logs error; no crash (already true). |
| Upgrade moves binary path | GUI regenerates unit from current `exec.LookPath("sway-power")` + `reexec` path check; worst case user reopens the app once. |

## Definition of done

- User flow: boot → log in → close lid → configured action runs, with
  sway-power never having been opened after first setup.
- First run: opening the app once enables the service from then on.
- `git grep` for `Monitor(` shows exactly two call sites:
  `internal/daemon` and tests. `git grep` for `systemd-inhibit` shows
  zero call sites in code (only docs/tests) — locking is pure D-Bus.

---

## Open investigation (deferred, keeps initial plan intact)

**"nothing" + external monitor: macOS-quality behavior.**

Current behavior already is: lid close + "nothing" + external connected →
`swaymsg output <eDP> disable`; lid open → re-enable. Open questions to
explore later (documenting here so the plan above doesn't drift):

1. **What exactly does macOS do?** macOS "Put display to sleep when the
   laptop lid is closed": with an external attached, the *internal*
   display goes dark and the external stays live; if no external is
   attached, the whole machine sleeps (not "nothing"). Verify whether the
   desired default on *no* external attached should be suspend rather
   than true-nothing.
2. **Re-open with external still attached:** today `showInternalDisplay`
   re-enables the internal panel even though the lid is closed again
   (external still connected). Decide: keep (matches "lid open = show my
   screen"), or only re-enable when no external is enabled.
3. **Mid-session monitor events:** currently only lid events are handled.
   If an external monitor is *plugged in* while "nothing" is active with
   the lid open, nothing changes (fine); if it's *unplugged* while lid is
   closed with "nothing", the machine is on with no display (fine — user
   intent is "don't suspend"). Verify behavior is sane for hotplug in both
   directions; may want a sway event subscription later rather than pure
   lid-event handling.
4. **Brightness/power:** macOS also dims/disables panels at the power
   level, not just output enable/disable. `swaymsg output <eDP> disable`
   is the closest sway primitive; confirm no better one exists
   (`output <eDP> brightness 0` keeps the backlight/scan-out alive and
   consumes slightly more power — probably fine to keep `disable`).

Nothing in phases 1–5 depends on this investigation; it only touches
`internal/lid/action/display.go` and possibly adds a fourth action
variant, which would then flow through the existing preferences file +
hot-reload with zero additional plumbing.
