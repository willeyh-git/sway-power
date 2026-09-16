# Implementation plan: set-and-forget lid handling (daemon + user service)

Status: plan v2 — incorporates lifecycle review (2024-xx). See "Review changes"
at the bottom for what changed and why.

## Problem

Today the lid monitor and the `systemd-inhibit` lock are children of the GUI
(`internal/ui/lid_buttons.go` → `lid.Monitor`). Closing the app releases the
inhibit and stops all handling, so lid behavior only exists while sway-power
is open. GNOME/KDE solve this by splitting a long-lived session daemon
(PowerDevil, gnome-power-manager) from the settings UI. We want the same
"set and forget" model: the app is only opened to *change* settings.

**Core invariant:** *the GUI has no authority over lid handling.* After first
setup the GUI's only systemd interactions are (optionally) reading service
status and, if the install is missing/broken, re-bootstrapping.

## Goals

- Lid handling (inhibit + state watching + actions) survives the GUI being
  closed, and starts/respawns with the session — no user intervention.
- No per-distro code paths. Requirement: works unmodified on Fedora and
  Arch (both systemd + logind). No `logind.conf` is read or written.
- Settings stay in `~/.config/sway-power/preferences.json` (already
  persisted), read/written atomically.
- Locking per https://systemd.io/INHIBITOR_LOCKS/: one
  `Inhibit("handle-lid-switch", …, "block")` D-Bus call on
  `org.freedesktop.login1.Manager`; lock lifetime = returned fd lifetime
  (kernel releases on crash). No `systemd-inhibit` process wrapper.

## Non-goals (out of scope for this pass)

- Expressing actions in `logind.conf` ("lock" and "nothing" are not
  expressible there; a daemon is the only way to keep all three actions).
- Improving the "nothing" + external-monitor display behavior. See
  **Open investigation** at the bottom.
- GUI readiness beyond service status in v1 (see "Daemon readiness" note).

## Target architecture

```
sway-power (GUI, on-demand)
  load/display prefs · bootstrap unit (once) · optional status
  │
  └── writes preferences.json (atomic: tmp + rename)
              │
              ▼
sway-power daemon (systemd --user, Restart=always,
                   WantedBy/PartOf=graphical-session.target)
  ┌─ Inhibitor          logind Inhibit() fd, retry until acquired
  ├─ Monitor            lid state, 500 ms poll
  ├─ PreferencesWatcher mtime poll, reload on change
  └─ Handler            current Action (atomic swap; action is NOT
                          re-bound on monitor restart — there is no
                          monitor restart)
```

Component responsibilities:

| Component | Owns | Never does |
|---|---|---|
| `Inhibitor` | the fd; acquire/retry/release | reads prefs, reacts to lid |
| `Monitor` | lid state, initial-state callback | executes anything |
| `PreferencesWatcher` | file mtime/poll, reload events | changes actions |
| `Handler` | current `action.Action` (swap under lock) | starts/stops monitor |

## Phases (ordered for independent, reviewable commits)

### 1. Extract the lid monitor from the GUI ✅ DONE

- Make `lid.Monitor` independently usable without the UI: move the
  closed/open → `Execute`/`OnOpen` mapping out of `lid_buttons.go` into a
  small reusable seam (this is the future `Handler`).
- No systemd, no D-Bus yet. Behavior unchanged: run the app as today and
  confirm identical behavior.

### 2. `sway-power daemon`: inhibitor + monitor + actions, headless ✅ DONE

- `cmd/sway-power/main.go`: subcommand dispatch (`daemon` vs GUI).
- New package `internal/daemon` with `Inhibitor`, `Handler`, (later
  `PreferencesWatcher`).
- **Inhibitor (recoverable state, not one-shot):**
  ```go
  lm, _ := dbus.NewLoginManagerFromSystemBus()   // github.com/systemd/systemd-go
  fd, err := lm.Inhibit("handle-lid-switch", "Sway Power",
      "Handle lid close ourselves", "block")
  ```
  - States: `acquiring → acquired | failed`.
  - On failure: log **prominently** and retry periodically (e.g. every 5 s
    with backoff) — D-Bus/logind can be briefly unavailable during session
    startup. Never permanently give up while the daemon lives.
  - On acquire: normal operation. While in `failed`, logind still handles
    lid — this is the documented double-handler window; it must be the
    exception, not the steady state, hence retry.
  - `os.Close(fd)` on SIGTERM/SIGINT exit; kernel closes on crash. No
    SIGTERM/kill/verify machinery — that all dies with `internal/lid`'s
    `startInhibit`/`Inhibitor`/`verifyInhibit`.
- Wire: `Monitor` → `Handler` → `action.Execute()` on `Closed`,
  `action.OnOpen()` on `Open`.
- **Initial-state rule:** the initial-state callback (lid already closed at
  daemon start → action fires once) happens **only at daemon startup**,
  never on config reload. Accept and document the consequence:
  daemon-crash → `Restart=always` → lid still closed → action fires again
  (e.g. suspend again). That matches "state on boot" and is rarer than it
  looks (suspend doesn't kill user services).
- Acceptance: run `sway-power daemon` in a sway session;
  `systemd-inhibit --list` shows "Sway Power"; lid close acts; SIGTERM →
  lock gone; `kill -9` → lock gone.

### 3. Config hot-reload = action swap (no monitor restart) ✅ DONE

- `PreferencesWatcher`: 1 s poll of `preferences.json` **mtime** (not
  inotify — see "Review changes"); on mtime change,
  `preferences.LoadFromPath()` + `Validate()`.
- On valid change: `Handler.SetAction(newAction)` — atomic swap under a
  mutex (or `atomic.Pointer[action]`). **The monitor is never restarted.**
- On parse/validate failure: keep last-good action, log.
- This makes the earlier "restart monitor on `setAction`" path in the GUI
  unnecessary; delete it.

### 4. Atomic preference writes ✅ DONE

- `preferences.Save()` currently does a plain `os.WriteFile` to the final
  path — the polling daemon can observe half-written JSON. Change to:
  write `preferences.json.tmp` in the same dir → `fsync` (best effort) →
  `rename` over `preferences.json`. Daemon then sees only complete files,
  which also makes the mtime-poll reload rule trivially correct (parse
  failure ⇒ retry next tick, never sticky).

### 5. systemd `--user` unit (added only once the daemon is stable) ✅ DONE

- `sway-power.service`:
  ```ini
  [Unit]
  Description=Sway Power lid handler (background)
  After=graphical-session.target
  PartOf=graphical-session.target

  [Service]
  ExecStart=<absolute stable path> daemon
  Restart=always
  RestartSec=2

  [Install]
  WantedBy=graphical-session.target
  ```
- **No `Environment=WAYLAND_DISPLAY=wayland-1`.** Instead:
  - Only child processes (`swaylock`, `swaymsg`) need the Wayland env; the
    daemon logic itself doesn't.
  - GUI bootstrap runs `systemctl --user import-environment
    WAYLAND_DISPLAY` (and, if handy, `XDG_SEAT`, `XDG_SESSION_ID`) so the
    user manager carries the *actual* session value — this also
    self-heals across sessions that use different display indices.
  - Verify per distro (see "Verify early"): compare
    `systemctl --user show-environment` with `env` inside a sway session.
- **Verify the Sway session integration before claiming "works on Fedora
  and Arch":** while sway is running,
  `systemctl --user status graphical-session.target` and
  `list-dependencies` on both distros. If the target isn't reliably
  active for the sway session, fall back to the mechanism the session
  actually uses — this is the part of the distro claim to validate first.
- `PartOf=graphical-session.target` is *a hypothesis* about logout
  behavior, not a known fact: integration test "logout → unit stopped,
  `systemd-inhibit --list` clean" explicitly (see Tests).

### 6. One-time bootstrap (GUI side), then status-only ✅ DONE

- First GUI launch performs install *once*:
  1. Write unit (if missing) with **absolute, stable** `ExecStart` path.
  2. `systemctl --user daemon-reload` (only if the file changed).
  3. `systemctl --user import-environment WAYLAND_DISPLAY` (idempotent).
  4. `systemctl --user enable --now sway-power` (idempotent).
- Normal GUI startup afterwards: load prefs, display, optional status
  line. It does **not** manage unit lifecycle.
- Upgrade handling, deliberately minimal: GUI compares the unit's
  `ExecStart` path with the currently running binary; if different,
  rewrite + reload + restart. (Preferred alternative if we ever package
  with a stable install location like `/usr/bin/sway-power`: the unit
  never changes at all — decide at implementation time; the compare-and-
  rewrite path is the safe default.)
- Document prominently: `systemctl --user status sway-power.service` is
  "the background lid handler", **not** the GUI. The executable, the
  `daemon` subcommand, and the unit are three related but distinct things.

### 7. GUI cleanup ✅ DONE

- `lid_buttons.go`: delete `startMonitor`, `stopWatch`, monitor start on
  `setAction`. `setAction()` = update buttons → atomic
  `preferences.Save()` → done.
- Status line (v1-min): "lid handler: active/inactive" via
  `systemctl --user is-active`.

## Daemon readiness (not v1, recorded)

`systemctl --user is-active` only proves the *process* is running, not
that the inhibitor was acquired (possible while `Inhibitor` is in
`failed`/retrying). Later, the GUI status becomes three lines:

```
Daemon:        active
Lid inhibitor: acquired | retrying (n)
Configuration: lock
```

Mechanism candidates: a small user-socket D-Bus object, a state file in
`$XDG_RUNTIME_DIR`, or `systemctl --user show Property=`. Deferred.

## Tests

**Unit** (`internal/daemon`, mirroring existing `*_test.go` style):
- Prefs → action resolution: valid / missing file / invalid value /
  unparseable file.
- Action-swap atomicity: swap under concurrent callback.
- **All tests are fully mocked** — no real commands (swaylock, systemctl,
  swaymsg) are executed during tests. The action package exports `Exec`
  and `SwaymsgCmd` function variables that tests override.

**Integration — inhibitor failure/recovery is the core correctness
property; test it explicitly:**
1. D-Bus/logind unavailable at daemon start → `failed` state, prominent
   log, periodic retry.
2. D-Bus becomes available → acquire, transition to normal.
3. SIGTERM → fd closed → `systemd-inhibit --list` clean.
4. `kill -9` → fd auto-closed (kernel) → lock gone; systemd restarts
   process → lock re-acquired.

**Integration — lifecycle (Fedora + Arch, both required):**
- Sway session running: `systemd-inhibit --list` shows "Sway Power".
- Close GUI → daemon unaffected; lid close still acts.
- Change setting via app → daemon swaps action (log), no restart.
- Daemon crash → `Restart=always` → back with lock.
- **Logout** → unit stopped, `systemd-inhibit --list` clean (this is the
  `PartOf`/target test — not assumed).
- Re-login → unit running without the app ever being opened.
- `systemctl --user show-environment` vs `env` in session → confirms
  `WAYLAND_DISPLAY` propagation (and what, if anything, is missing).
- First-run: delete unit → open app once → unit present, enabled, active.

**Definition of done:**
- Boot → log in → close lid → configured action runs, app never opened
  after first setup.
- `git grep Monitor(` → exactly `internal/daemon` + tests.
- `git grep systemd-inhibit` → zero call sites in code.

## Risks / edge cases

| Case | Handling |
|---|---|
| `Inhibitor` in `failed` (denied or D-Bus down) | Log prominently, retry on a timer until acquired; while failed, double-handler window with logind is possible — accepted as transient, must not be steady state. |
| `Inhibit()` denied by polkit permanently | Same retry; additionally, since this is a user-initiated block-lid-switch lock and both target distros allow it by default, treat persistent denial as "wrong environment" and keep retrying (cheap). |
| GUI opened twice before first-run bootstrap | Bootstrap steps are idempotent; add a file lock (`flock` on `preferences.json.lock` or `XDG_RUNTIME_DIR`) around the write/reload/enable sequence to avoid interleaved unit writes. |
| Non-systemd session (no logind) | Daemon logs "inhibit unavailable", retries, lid actions still fire on state — degraded but alive. Out of target distros (Fedora/Arch always have both). |
| Config file hand-edited to invalid value | Last-good action kept, error logged, watcher retries next mtime change. |
| `swaylock`/`swaymsg` missing | Action logs error; daemon continues (already true today). |
| Upgrade moves binary path | GUI bootstrap compare-and-rewrite (phase 6); worst case user reopens the app once. |
| Editor writes prefs via tmp+rename itself | mtime-poll + parse-on-read handles it; atomic `Save()` in phase 4 makes our own writer safe. |

## Verify early (before committing to "Fedora + Arch unmodified")

1. `systemctl --user status graphical-session.target` +
   `list-dependencies` **inside a running sway session** on both distros.
   If not reliably active there, re-baseline the unit's integration
   target to whatever the session actually uses.
2. `systemctl --user show-environment` vs `env` in the session — decides
   exactly what `import-environment` must carry.
3. `sd-inhibit`/polkit: confirm user-level
   `org.freedesktop.login1.inhibit-block-handle-lid-switch` is allowed by
   default policy on both distros (it is, stock; confirm).
4. Logout behavior: does the user manager stop `PartOf` units? (covered by
   lifecycle test; record result.)

---

## Review changes (v1 → v2)

- **Action swap, not monitor restart** (review): `Handler` owns an
  atomically-swappable `action`; `PreferencesWatcher` never restarts the
  monitor. Simpler lifecycle, no stop/start race with an arriving lid
  event. Initial-state callback restricted to daemon startup.
- **Inhibitor is a recoverable state machine** (review): acquire →
  acquired | failed; retry while failed; prominent logging. v1's "log and
  degrade" understated the logind double-handler window.
- **No hardcoded `WAYLAND_DISPLAY`** (review): `import-environment` at
  bootstrap + "verify early" item 2. Only child processes need it.
- **One-time bootstrap** (review): GUI installs the unit once; normal
  startup is load/display/status only. Upgrade = compare-and-rewrite
  ExecStart path (stable absolute path preferred if packaging gives one).
- **Poll, not inotify** (review): 1 s mtime poll; also survives
  editor tmp+rename patterns; parse-on-read rule.
- **Atomic `preferences.Save()`** (new, phase 4): tmp + fsync + rename.
  Current code is a plain `os.WriteFile` — verified while re-planning.
- **Reordered phases** (review): extract monitor → daemon (inhibit +
  monitor + actions, headless) → action-swap reload → atomic writes →
  unit → bootstrap → GUI cleanup. Each commit independently reviewable.
- **`graphical-session.target` / `PartOf` downgraded from assumption to
  "verify early"** (review): both distros, in-session, before the
  "works unmodified" claim stands.
- **Test list expanded** (review): inhibitor failure/recovery sequence
  added as the primary correctness test; logout = explicit `PartOf`
  test; service naming documented (unit ≠ GUI).
- **Daemon readiness** added as a *deferred* item (review, "not v1"):
  `is-active` ≠ "inhibitor acquired"; three-line status design recorded
  for later.

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

Nothing in phases 1–7 depends on this investigation; it only touches
`internal/lid/action/display.go` and possibly adds a fourth action
variant, which would then flow through the existing preferences file +
action-swap with zero additional plumbing.
