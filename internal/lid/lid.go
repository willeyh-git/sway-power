package lid

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
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
func Monitor(fn func(State)) func() {
	// Find the lid switch path.
	lidPath := findLidState()
	if lidPath == "" {
		return func() {}
	}

	// Inhibit systemd-logind from handling the lid switch before we react
	// to any state, so logind never races us on startup.
	inhibitor, err := startInhibit()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[lid] failed to inhibit logind: %v\n", err)
	}

	stopCh := make(chan struct{})

	// Read initial state.
	initial, err := readLidState(lidPath)
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
				current, err := readLidState(lidPath)
				if err != nil {
					fmt.Fprintf(os.Stderr, "[lid] error reading lid state: %v\n", err)
					continue
				}
				if current != last {
					fmt.Fprintf(os.Stderr, "[lid] lid state changed: %s\n", current)
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
}

// startInhibit starts systemd-inhibit to block logind from handling the
// lid switch. Using systemd-inhibit (instead of a logind.conf override)
// keeps this distro agnostic: it works on Arch, Fedora, etc. alike.
func startInhibit() (*Inhibitor, error) {
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

	inh := &Inhibitor{cmd: cmd, done: make(chan struct{})}
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
	_ = i.cmd.Process.Kill()
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
func findLidState() string {
	// Try /proc/acpi/button/lid/*/state first.
	dirs, err := filepath.Glob("/proc/acpi/button/lid/*/state")
	if err == nil && len(dirs) > 0 {
		fmt.Fprintf(os.Stderr, "[lid] found lid state at %s\n", dirs[0])
		return dirs[0]
	}

	// Try /sys/class/input/event*/device/lid_switch.
	dirs, err = filepath.Glob("/sys/class/input/*/device/lid_switch")
	if err == nil && len(dirs) > 0 {
		dir := filepath.Dir(dirs[0])
		statePath := filepath.Join(dir, "state")
		if _, err := os.Stat(statePath); err == nil {
			fmt.Fprintf(os.Stderr, "[lid] found lid state at %s\n", statePath)
			return statePath
		}
	}

	return ""
}

// readLidState reads the lid state from the given path.
func readLidState(path string) (State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read lid state: %w", err)
	}

	state := strings.ToLower(string(data))
	fmt.Fprintf(os.Stderr, "[lid] read state: %q\n", strings.TrimSpace(string(data)))

	if strings.Contains(state, "open") {
		return Open, nil
	}
	if strings.Contains(state, "close") {
		return Closed, nil
	}

	return 0, fmt.Errorf("unknown lid state: %q", strings.TrimSpace(string(data)))
}
