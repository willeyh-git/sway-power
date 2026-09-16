package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LidState represents the current lid state.
type LidState int

const (
	LidOpen   LidState = iota // Lid is open
	LidClosed                 // Lid is closed
)

func (s LidState) String() string {
	switch s {
	case LidOpen:
		return "open"
	case LidClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// Monitor watches the lid switch state.
type Monitor struct {
	lidPath  string
	log      Logger
	debug    bool
	callback func(LidState) // called on state change or initial state
	stopCh   chan struct{}
	done     chan struct{}
}

// NewMonitor creates a new lid state monitor.
// The callback is called immediately with the initial state, then on each change.
// The monitor polls at 500ms intervals; debug enables the chatty per-poll
// read log.
func NewMonitor(log Logger, debug bool, callback func(LidState)) *Monitor {
	lidPath := findLidState(log)
	if lidPath == "" {
		log.Printf("lid: could not find lid state file")
		return nil
	}

	m := &Monitor{
		lidPath:  lidPath,
		log:      log,
		debug:    debug,
		callback: callback,
		stopCh:   make(chan struct{}),
		done:     make(chan struct{}),
	}

	// Start monitoring (reads initial state internally).
	go m.watch()

	return m
}

// watch polls for lid state changes.
func (m *Monitor) watch() {
	defer close(m.done)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	// Read initial state.
	initial, err := m.readLidState()
	if err != nil {
		m.log.Printf("lid: could not read initial state: %v", err)
		return
	}
	m.log.Printf("lid: initial state: %s", initial)
	m.callback(initial)

	last := initial

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			current, err := m.readLidState()
			if err != nil {
				m.log.Printf("lid: error reading state: %v", err)
				continue
			}
			if current != last {
				m.log.Printf("lid: state changed: %s", current)
				last = current
				m.callback(current)
			}
		}
	}
}

// Stop stops the monitor.
func (m *Monitor) Stop() {
	select {
	case <-m.stopCh:
		return
	default:
	}
	close(m.stopCh)
	<-m.done
}

// findLidState finds the path to the lid state file.
func findLidState(log Logger) string {
	// Try /proc/acpi/button/lid/*/state first.
	dirs, err := filepath.Glob("/proc/acpi/button/lid/*/state")
	if err == nil && len(dirs) > 0 {
		log.Printf("lid: found at %s", dirs[0])
		return dirs[0]
	}

	// Try /sys/class/input/event*/device/lid_switch.
	dirs, err = filepath.Glob("/sys/class/input/*/device/lid_switch")
	if err == nil && len(dirs) > 0 {
		dir := filepath.Dir(dirs[0])
		statePath := filepath.Join(dir, "state")
		if _, err := os.Stat(statePath); err == nil {
			log.Printf("lid: found at %s", statePath)
			return statePath
		}
	}

	return ""
}

// readLidState reads the lid state from the monitored path. The raw file
// content is logged only in debug mode; in normal operation the monitor
// is steady-state (no per-poll journal noise) and only state changes and
// errors are logged.
func (m *Monitor) readLidState() (LidState, error) {
	data, err := os.ReadFile(m.lidPath)
	if err != nil {
		return 0, err
	}

	state := strings.ToLower(string(data))
	if m.debug {
		m.log.Printf("lid: read %q", strings.TrimSpace(string(data)))
	}

	if strings.Contains(state, "open") {
		return LidOpen, nil
	}
	if strings.Contains(state, "close") {
		return LidClosed, nil
	}

	return 0, fmt.Errorf("unknown lid state: %q", strings.TrimSpace(string(data)))
}
