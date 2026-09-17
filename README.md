# sway-power

Power widget for the [sway](https://swaywm.org/) Wayland compositor. A small
window app that shows battery status, switches power profiles, and
sets what happens when you close the laptop lid — with lid handling that
keeps working after the app is closed.

## Features

- **Battery display** — charge level, time estimate, and charge cycles, read
  from `/sys/class/power_supply`.
- **Power profiles** — Eco / Normal / Performance buttons. Auto-detects the
  running daemon: `tuned-ppd` (system bus, Fedora default) or
  `power-profiles-daemon` (session bus, Arch/Debian). If neither is
  present, the buttons are inert and a status line says so.
- **Lid close action** — choose what closing the lid does:
  - `lock` (default) — run `swaylock -f`
  - `sleep` — `systemctl suspend`
  - `nothing` — don't suspend; instead turn off the internal display when an
    external monitor is connected (so the machine stays usable), and re-enable
    it when the lid opens

The defining behavior is in the last one: **lid handling is set-and-forget.**
The GUI only *changes* settings. Once the background daemon is installed,
lid handling survives the app being closed and starts/respawns with your
graphical session.

## How it works

Two programs, one binary:

| Program | Role |
|---|---|
| `sway-power` (GUI) | Load/display preferences, set the lid action, show status. Has **no authority** over lid handling after first setup. |
| `sway-power daemon` | Headless background service: holds the logind inhibit lock, watches the lid, and executes the configured action. |

The daemon owns four components:

- **Inhibitor** — takes a logind lock, `Inhibit("handle-lid-switch", …, "block")`
  over D-Bus, so logind doesn't also handle the lid. The lock's lifetime is
  the returned file descriptor's: closing the fd releases it, and the kernel
  releases it if the process dies. If the bus/logind is unavailable or the
  call is denied, the inhibitor retries every 5 s and never gives up while
  the daemon lives.
- **Monitor** — consumes the lid switch's evdev input device
  (`/dev/input/eventN`, discovered via the `/sys/class/input/inputN`
  class entry whose `name` is `Lid Switch`) for zero-latency transitions,
  and reads the state
  file (`/proc/acpi/button/lid/*/state` or
  `/sys/class/input/*/device/lid_switch`) once for the initial state and
  every 500 ms as a backstop, so a missed event can never leave the daemon
  out of sync. Reports changes: once at startup with the initial state (if
  the lid is already closed, the action fires once), then on each change.
- **Preferences watcher** — polls `preferences.json` mtime every second and,
  on a valid change, **atomically swaps** the running action. The monitor is
  never restarted. On a corrupt file the last-good action is kept.
- **Handler** — holds the current action and executes it on lid close
  (`Execute`) / lid open (`OnOpen`).

### Lid service install (explicit)

Opening the GUI installs nothing. The background lid handler runs as the
systemd **user** service `sway-power.service`, and installing it is an
explicit action: `sway-power install`, or the "Install the lid service"
link in the GUI's Lid Settings section. Both do:

1. write the unit (with the absolute path of the running binary) to
   `~/.config/systemd/user/`,
2. `systemctl --user daemon-reload` + `import-environment` (so the daemon's
   `swaylock`/`swaymsg` children see `WAYLAND_DISPLAY`),
3. `systemctl --user enable --now sway-power`.

If the unit is unchanged, install rewrites nothing, but if the service is
**running** it restarts it, so an in-place binary upgrade (same path,
new binary) takes effect instead of waiting for the next login. A daemon you deliberately stopped stays stopped. If you
rebuild and the binary path changes, re-run install: it rewrites the unit
and restarts the service either way. `sway-power uninstall` (or the GUI link) stops
the service, disables it, and removes the unit.

## Requirements

- Linux with **sway** (or a Wayland compositor) and a laptop lid switch
- **systemd** + **logind** (user session) — target distros are **Fedora** and
  **Arch**
- `swaylock` and `swaymsg` (for the `lock` and `nothing` actions)
- optional: `power-profiles-daemon` or `tuned-ppd` (for the power-profile
  buttons)
- to build: **Go 1.27.1+**, and for the GUI a C toolchain plus the usual
  Fyne desktop dependencies (X11/Wayland headers)

### Sway / systemd session requirement

The daemon is a systemd **user** service whose child processes (`swaylock`,
`swaymsg`, `systemctl`) must be able to reach the **running Sway session**.
That requires all of the following:

- you are actually *in* a Sway session — installing sway is not enough; the
  session the daemon inherits must be a live Wayland/Sway one
- the systemd **user** manager is running and your session reaches
  `graphical-session.target` (the unit's integration target). Plain Sway
  installations launched outside the usual display-session integration are
  **not** guaranteed to reach it; check from inside your session with
  `systemctl --user status graphical-session.target`
- the session environment the child processes need is propagated to the
  user manager:
  - `WAYLAND_DISPLAY` — `swaylock`/`swaymsg` address the compositor through
    it. Install runs `systemctl --user import-environment` with it;
    if you ever (re)start the daemon from a different shell than your Sway
    session, re-import it:
    `systemctl --user import-environment WAYLAND_DISPLAY`
  - `XDG_RUNTIME_DIR` — the `systemctl --user` manager and sway's IPC
    socket live under it; it is set by the systemd user session itself

At startup the daemon checks exactly these and logs a prominent
`session:`-prefixed warning for each missing piece (unset `WAYLAND_DISPLAY`,
unset `XDG_RUNTIME_DIR`, `XDG_SESSION_TYPE` set to something other than
`wayland`, or `swaylock`/`swaymsg`/`systemctl` not on `PATH`). It does **not**
exit on them — each affected action then fails with its own error — but the
startup warnings make a broken session diagnosable instead of silently
degraded:

```sh
journalctl --user -u sway-power | grep 'session:'   # startup session warnings
systemctl --user show-environment                    # what the user manager has
systemctl --user status graphical-session.target     # is the unit even started?
```

## Lid switch discovery (hardware compatibility)

The monitor discovers the lid switch at startup and logs what it found:

| Source | Location | Role | Verified |
|---|---|---|---|
| evdev input device | `/dev/input/eventN`, where `inputN` is the `/sys/class/input` class entry whose `name` is `Lid Switch` (same index `N`) | primary: real input event stream (`SW_LID`, 1 = open, 0 = closed) | yes — `input0` on the verified machine maps to `/dev/input/event0` (`capabilities/sw` exposes `SW_LID`) |
| ACPI proc interface | `/proc/acpi/button/lid/*/state` (`open` / `closed`) | initial state + 500 ms polling backstop | yes — `LID0/state` present, format `state:      open` |
| sysfs attribute | `/sys/class/input/*/device/lid_switch` (`1` / `0`) | fallback state file for hardware that exposes the attribute via the input class | not observed on the verified machine (its attribute lives under the platform device, not the input class); probed in order |

The verified machine has both an ACPI proc interface and the evdev device,
which is the common laptop configuration. If **no** source is found the
monitor logs `lid: could not find lid switch`, disables itself, and logind
keeps handling the lid — the daemon still runs (inhibitor included).

Note on the sysfs row: the pre-rewrite code looked for a `state` file next
to the `lid_switch` attribute, which no hardware has (the attribute itself
carries the 0/1 state) — that probe was dead. The monitor now reads the
attribute directly.

## Build

```sh
make build               # = go build -trimpath -ldflags "-X main.version=…";
                         # version from `git describe` (the git tag)
```

(Plain `go build -o sway-power ./cmd/sway-power` also works, but the binary
then reports `version: dev` — the Makefile target is the one that matches
releases.)

The GUI is built with [Fyne](https://fyne.io/), so the desktop build needs a
C compiler and the X11/Wayland development libraries. Typical packages:

- Fedora: `sudo dnf install gcc make wayland-devel`
- Arch: `sudo pacman -S base-devel wayland`
- Debian/Ubuntu: `sudo apt install gcc libgl1-mesa-dev xorg-dev`

## Usage

```sh
sway-power                 # the GUI
sway-power install         # install + start the lid handler service
sway-power uninstall       # stop, disable, and remove the lid handler service
sway-power daemon          # the headless daemon (normally run by systemd, not by you)
sway-power -debug daemon   # -debug goes before the subcommand
sway-power --version       # the build version (embedded from the git tag)
```

`-debug` logs D-Bus activity and per-poll diagnostics to stderr. The daemon
always logs to stderr (→ the journal under systemd) regardless, so failures
are visible:

```sh
journalctl --user -u sway-power -f
```

### Managing the background service

```sh
systemctl --user status sway-power     # is the lid handler running?
systemctl --user stop sway-power       # pause lid handling (GUI can't touch it)
systemctl --user start sway-power      # resume
systemctl --user disable --now sway-power   # stop and never auto-start again
systemd-inhibit --list                 # should show a sway-power entry on handle-lid-switch, mode block
```

Note the distinction: the **executable**, the `daemon` **subcommand**, and the
**`sway-power.service` unit** are three related but distinct things. The unit
is the background lid handler, *not* the GUI.

## Configuration

- **Lid action** — `~/.config/sway-power/preferences.json`
  (`{"lid_close": "lock" | "sleep" | "nothing"}`). Written atomically by the
  GUI; hot-reloaded by the daemon without a restart.
- **Theme** — `~/.config/sway-power/config.yaml`. Optional overrides for
  light/dark mode and each color (accent, background, label, button states,
  …). Any field left empty falls back to the built-in palette. See
  `internal/config/config.go` for the full list.
- **Unit file** — `~/.config/systemd/user/sway-power.service` (managed by `sway-power install`/`uninstall`; the template lives at
  `internal/bootstrap/sway-power.service`).

## Project layout

```
cmd/sway-power/        entry point: GUI vs `daemon`/`install`/`uninstall`
                       subcommands
internal/
  ui/                  Fyne window: battery, power profiles, lid buttons
  daemon/              the background lid handler (inhibitor, monitor,
                       preferences watcher, handler)
  bootstrap/           explicit systemd user-service install/uninstall
  lid/action/          lock / sleep / nothing action implementations
  power/               power-profiles-daemon / tuned-ppd client
  battery/             /sys/class/power_supply reader
  preferences/         preferences.json persistence (atomic writes)
  config/              config.yaml loading + validation
  logger/              stderr logger (debug-gated)
```

## Development

```sh
make test              # unit tests, with -race — mandatory: the daemon has
                       # concurrent lifecycles (inhibitor, monitor, prefs
                       # watcher) and only the race detector covers them;
                       # fully mocked, no real commands run;
                       # TestMonitorRealLidSource additionally probes this
                       # machine's lid switch (skips when absent)
make vet               # go vet ./...
```

`make build` / `make release` embed the version from `git describe`
(`-ldflags -X main.version=…`), so `sway-power --version` and the daemon's
`daemon: starting (sway-power …)` journal line report the build.

The inhibitor's retry/recovery behavior is the core correctness property
of the lid architecture and is what the tests focus on.

Before refactoring the daemon, read [docs/invariants.md](docs/invariants.md):
the properties the code must never violate (inhibitor, display ownership,
action swap, monitor delivery, preference loading).
