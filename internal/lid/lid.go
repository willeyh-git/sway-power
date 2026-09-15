package lid

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// State represents the current lid state.
type State int

const (
	Open  State = iota // Lid is open
	Closed             // Lid is closed
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
// It returns a function to stop monitoring.
func Monitor(fn func(State)) func() {
	// Find the lid switch path.
	lidPath := findLidState()
	if lidPath == "" {
		return func() {}
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
					continue
				}
				if current != last {
					last = current
					fn(current)
				}
			}
		}
	}()

	return func() {
		close(stopCh)
	}
}

// findLidState finds the path to the lid state file.
// Tries common locations.
func findLidState() string {
	// Try /proc/acpi/button/lid/*/state first.
	dirs, err := filepath.Glob("/proc/acpi/button/lid/*/state")
	if err == nil && len(dirs) > 0 {
		return dirs[0]
	}

	// Try /sys/class/input/event*/device/lid_switch.
	dirs, err = filepath.Glob("/sys/class/input/*/device/lid_switch")
	if err == nil && len(dirs) > 0 {
		// Read the state file next to it.
		dir := filepath.Dir(dirs[0])
		statePath := filepath.Join(dir, "state")
		if _, err := os.Stat(statePath); err == nil {
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

	state := strings.TrimSpace(strings.ToLower(string(data)))

	if strings.Contains(state, "open") || strings.Contains(state, "0") {
		return Open, nil
	}
	if strings.Contains(state, "close") || strings.Contains(state, "1") {
		return Closed, nil
	}

	return 0, fmt.Errorf("unknown lid state: %q", state)
}
