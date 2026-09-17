// Package bootstrap handles one-time installation of the sway-power
// systemd user service.
package bootstrap

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// unitName is the systemd user unit managed by this package.
const unitName = "sway-power.service"

// unitTemplate is the unit file installed at bootstrap; __EXEC_PATH__ is
// replaced with the absolute path to the running sway-power binary.
//
//go:embed sway-power.service
var unitTemplate string

// Bootstrap installs the sway-power systemd user service. It is an
// explicit user action (GUI link or `sway-power install`), never a side
// effect of a GUI launch. execPath must
// be the absolute path to the running sway-power binary; it goes into
// the unit's ExecStart line.
//
//   - unit missing   -> write, daemon-reload, import-environment,
//     enable --now (first-run install).
//   - unit changed   -> rewrite, daemon-reload, import-environment,
//     enable, restart (upgrade path: the running daemon picks up the
//     new ExecStart path).
//   - unit unchanged -> nothing. Normal GUI startup never manages unit
//     lifecycle: a daemon the user deliberately stopped stays stopped.
//
// The whole write/reload/enable sequence runs under an exclusive flock
// so two concurrent first GUI launches cannot interleave it.
func Bootstrap(execPath string) error {
	lockFile, err := acquireLock()
	if err != nil {
		return err
	}
	defer releaseLock(lockFile)
	return doBootstrap(execPath)
}

// UnitName returns the name of the managed user unit.
func UnitName() string {
	return unitName
}

// UnitPath returns the path of the managed user unit.
func UnitPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("bootstrap: user config dir: %w", err)
	}
	return filepath.Join(configDir, "systemd", "user", unitName), nil
}

// IsInstalled reports whether the managed user unit file is present.
func IsInstalled() bool {
	p, err := UnitPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// Uninstall removes the managed user unit: stop + disable (best effort),
// delete the unit file, daemon-reload. Idempotent: a missing unit is not
// an error.
func Uninstall() error {
	lockFile, err := acquireLock()
	if err != nil {
		return err
	}
	defer releaseLock(lockFile)

	unitPath, err := UnitPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(unitPath); os.IsNotExist(err) {
		return nil // already uninstalled
	}

	// Best effort: the unit may be inactive or unknown to the user manager.
	_ = systemctl("disable", "--now", unitName)

	if err := os.Remove(unitPath); err != nil {
		return fmt.Errorf("bootstrap: remove unit: %w", err)
	}
	return systemctl("daemon-reload")
}

// doBootstrap is Bootstrap with the lock already held.
func doBootstrap(execPath string) error {
	unitPath, err := UnitPath()
	if err != nil {
		return err
	}
	unitDir := filepath.Dir(unitPath)

	unitContent := strings.Replace(unitTemplate, "__EXEC_PATH__", execPath, 1)

	// Read the installed unit, if any.
	var installed string
	exists := false
	if data, err := os.ReadFile(unitPath); err == nil {
		installed = string(data)
		exists = true
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("bootstrap: read unit: %w", err)
	}

	if exists && installed == unitContent {
		// Unit is up to date. Normal startup does not manage unit
		// lifecycle here (plan phase 6): a daemon the user
		// deliberately stopped stays stopped.
		return nil
	}

	// First run or upgrade: write the unit file.
	if err := os.MkdirAll(unitDir, 0755); err != nil {
		return fmt.Errorf("bootstrap: create unit dir: %w", err)
	}
	if err := os.WriteFile(unitPath, []byte(unitContent), 0644); err != nil {
		return fmt.Errorf("bootstrap: write unit: %w", err)
	}

	// Propagate session env (WAYLAND_DISPLAY, ...) to the user manager.
	// Non-fatal: only the daemon's child processes need it.
	_ = importEnvironment()

	if err := systemctl("daemon-reload"); err != nil {
		return fmt.Errorf("bootstrap: daemon-reload: %w", err)
	}

	if !exists {
		// First run: enable and start.
		return systemctl("enable", "--now", unitName)
	}

	// Upgrade: keep it enabled, and restart so the new unit (ExecStart
	// path) is what actually runs — otherwise the old binary keeps
	// serving until next login.
	if err := systemctl("enable", unitName); err != nil {
		return fmt.Errorf("bootstrap: enable: %w", err)
	}
	return systemctl("restart", unitName)
}

// acquireLock takes an exclusive advisory lock over the bootstrap
// sequence (plan risk table: two concurrent first GUI launches must not
// interleave write/reload/enable).
//
// The lock lives in $XDG_RUNTIME_DIR when available (session-scoped,
// cleaned up on logout) and in the user config dir otherwise. The file
// is left in place: it is the lock, not state.
func acquireLock() (*os.File, error) {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		var err error
		dir, err = os.UserConfigDir()
		if err != nil {
			return nil, fmt.Errorf("bootstrap: no lock dir: %w", err)
		}
		dir = filepath.Join(dir, "sway-power")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("bootstrap: create lock dir: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "sway-power-bootstrap.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: open lock file: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("bootstrap: flock: %w", err)
	}
	return f, nil
}

func releaseLock(f *os.File) {
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	f.Close()
}

// importEnvironment runs systemctl --user import-environment to
// propagate WAYLAND_DISPLAY and other session variables to the user
// manager.
func importEnvironment() error {
	env := os.Environ()
	var vars []string
	for _, e := range env {
		if strings.HasPrefix(e, "WAYLAND_DISPLAY=") ||
			strings.HasPrefix(e, "XDG_SEAT=") ||
			strings.HasPrefix(e, "XDG_SESSION_ID=") {
			vars = append(vars, strings.SplitN(e, "=", 2)[0])
		}
	}
	if len(vars) == 0 {
		return nil // nothing to import
	}

	cmd := exec.Command("systemctl", "--user", "import-environment")
	cmd.Args = append(cmd.Args, vars...)
	return cmd.Run()
}

// systemctl runs systemctl --user with the given arguments.
func systemctl(args ...string) error {
	fullArgs := append([]string{"--user"}, args...)
	cmd := exec.Command("systemctl", fullArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s: %w: %s", strings.Join(fullArgs, " "), err, string(out))
	}
	return nil
}
