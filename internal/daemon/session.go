package daemon

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/willeyh-git/sway-power/internal/logger"
)

// SessionWarnings returns one warning per missing piece of the Sway/systemd
// session environment the daemon's child processes rely on:
//
//   - WAYLAND_DISPLAY: swaylock and swaymsg address the compositor through
//     it; without it the "lock" and "nothing" actions can never work.
//   - XDG_RUNTIME_DIR: the systemd --user manager and sway's IPC socket
//     live under it; without it "sleep" cannot reach systemctl.
//   - XDG_SESSION_TYPE: when set but not "wayland", the daemon is probably
//     not running under the Sway session it should inherit.
//   - swaylock / swaymsg / systemctl on PATH.
//
// An empty slice means the session looks complete. The daemon never fails
// because of this: the warnings are logged prominently at startup (see
// checkSession) so a broken session is diagnosable from the journal
// instead of silently degrading, and each affected action then fails
// later with its own error.
func SessionWarnings() []string {
	var warnings []string

	if os.Getenv("WAYLAND_DISPLAY") == "" {
		warnings = append(warnings,
			`WAYLAND_DISPLAY is not set: swaylock and swaymsg cannot find the compositor, so the "lock" and "nothing" actions will fail. The daemon must run inside a Sway session, or the variable must be propagated to the user manager with: systemctl --user import-environment WAYLAND_DISPLAY`)
	}
	if os.Getenv("XDG_RUNTIME_DIR") == "" {
		warnings = append(warnings,
			`XDG_RUNTIME_DIR is not set: systemctl --user and sway's IPC socket are unreachable, so the "sleep" action cannot run. The daemon must run inside a systemd user session`)
	}
	if s := os.Getenv("XDG_SESSION_TYPE"); s != "" && s != "wayland" {
		warnings = append(warnings, fmt.Sprintf(
			`XDG_SESSION_TYPE=%q, expected "wayland": the daemon is probably not running under the Sway session`, s))
	}
	for _, bin := range []string{"swaylock", "swaymsg", "systemctl"} {
		if _, err := exec.LookPath(bin); err != nil {
			warnings = append(warnings, fmt.Sprintf(
				`%s not found in PATH: the actions that shell out to it will fail`, bin))
		}
	}
	return warnings
}

// checkSession logs a prominent "session:" warning for every missing piece
// of the session environment. Called once during daemon construction in
// New, before any action can run, so the warnings reach the journal ahead
// of the first lid event.
func checkSession(lg *logger.Logger) {
	for _, w := range SessionWarnings() {
		lg.Printf("session: %s", w)
	}
}
