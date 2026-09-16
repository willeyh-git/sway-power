// Package lid monitors lid open/close state and provides a suspend inhibitor.
package lid

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/willeyh-git/sway-power/internal/logger"
)

// State represents the current lid state.
type State int

const (
	Open   State = iota // Lid is open
	Closed              // Lid is closed
)

func (s State) String() string {
	switch s {
	case Open:
		return "open"
	case Closed:
		return "closed"
	default:
		return "unknown"
	}
}

// Monitor watches the lid switch state and calls fn whenever it changes.
// It inhibits systemd-logind from handling the lid switch (via
// systemd-inhibit, which works on any systemd distro without overriding
// logind.conf) so we can control what happens on lid close ourselves.
// Returns a function to stop monitoring and release the inhibit.
func Monitor(fn func(State), lg *logger.Logger) func() {
	// Find the lid switch path.
	lidPath := findLidState(lg)
	if lidPath == "" {
		return func() {}
	}

	// Inhibit systemd-logind from handling the lid switch before we react
	// to any state, so logind never races us on startup.
	inhibitor, err := startInhibit(lg)
	if err != nil {
		lg.Printf("failed to inhibit logind: %v", err)
	}

	stopCh := make(chan struct{})

	// Read initial state.
	initial, err := readLidState(lidPath, lg)
	if err == nil {
		fn(initial)
	}

	// Watch for changes.
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		var last State
		if err == nil {
			last = initial
		}

		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				current, err := readLidState(lidPath, lg)
				if err != nil {
					lg.Printf("error reading lid state: %v", err)
					continue
				}
				if current != last {
					lg.Printf("lid state changed: %s", current)
					last = current
					fn(current)
				}
			}
		}
	}()

	// Return cleanup function.
	return func() {
		close(stopCh)
		inhibitor.Release()
	}
}

// Inhibitor holds a systemd-inhibit process that blocks logind from
// handling the lid switch.
type Inhibitor struct {
	cmd  *exec.Cmd
	done chan struct{}
	log  *logger.Logger
}

// startInhibit starts systemd-inhibit to block logind from handling the
// lid switch. Using systemd-inhibit (instead of a logind.conf override)
// keeps this distro agnostic: it works on Arch, Fedora, etc. alike.
func startInhibit(lg *logger.Logger) (*Inhibitor, error) {
	cmd := exec.Command("systemd-inhibit",
		"--what=handle-lid-switch",
		"--who=Sway Power",
		"--why=Handle lid close ourselves",
		"--mode=block",
		"/bin/sleep", "infinity")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start systemd-inhibit: %w", err)
	}

	inh := &Inhibitor{cmd: cmd, done: make(chan struct{}), log: lg}
	go func() {
		cmd.Wait()
		close(inh.done)
	}()

	// Give it a moment to register.
	time.Sleep(100 * time.Millisecond)

	// Verify the inhibit was successful.
	if err := verifyInhibit(); err != nil {
		inh.Release()
		return nil, fmt.Errorf("verify inhibit: %w", err)
	}

	return inh, nil
}

// Release terminates systemd-inhibit and releases the inhibit lock.
func (i *Inhibitor) Release() {
	if i == nil {
		return
	}

	// SIGTERM (not SIGKILL) so systemd-inhibit can terminate its child
	// command and release the lock cleanly.
	if err := i.cmd.Process.Signal(syscall.SIGTERM); err == nil {
		select {
		case <-i.done:
			return
		case <-time.After(2 * time.Second):
		}
	}
	if err := i.cmd.Process.Kill(); err != nil {
		i.log.Printf("failed to kill inhibit process: %v", err)
	}
	<-i.done
}

// verifyInhibit checks that systemd-inhibit is blocking logind.
func verifyInhibit() error {
	cmd := exec.Command("systemd-inhibit", "--list", "--what=handle-lid-switch", "--no-pager")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("list inhibitors: %w", err)
	}
	if !strings.Contains(string(out), "Sway Power") {
		return fmt.Errorf("inhibit not registered")
	}
	return nil
}

// findLidState finds the path to the lid state file.
func findLidState(lg *logger.Logger) string {
	// Try /proc/acpi/button/lid/*/state first.
	dirs, err := filepath.Glob("/proc/acpi/button/lid/*/state")
	if err == nil && len(dirs) > 0 {
		lg.Printf("found lid state at %s", dirs[0])
		return dirs[0]
	}

	// Try /sys/class/input/event*/device/lid_switch.
	dirs, err = filepath.Glob("/sys/class/input/*/device/lid_switch")
	if err == nil && len(dirs) > 0 {
		dir := filepath.Dir(dirs[0])
		statePath := filepath.Join(dir, "state")
		if _, err := os.Stat(statePath); err == nil {
			lg.Printf("found lid state at %s", statePath)
			return statePath
		}
	}

	return ""
}

// readLidState reads the lid state from the given path.
func readLidState(path string, lg *logger.Logger) (State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read lid state: %w", err)
	}

	state := strings.ToLower(string(data))
	lg.Printf("read state: %q", strings.TrimSpace(string(data)))

	if strings.Contains(state, "open") {
		return Open, nil
	}
	if strings.Contains(state, "close") {
		return Closed, nil
	}

	return 0, fmt.Errorf("unknown lid state: %q", strings.TrimSpace(string(data)))
}
