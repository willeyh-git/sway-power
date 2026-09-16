// Package power provides a client for power-profiles-daemon via D-Bus.
// It supports the three standard profiles: power-saver, balanced, performance.
//
// Auto-detects the running daemon:
//   - Fedora: tuned-ppd (system bus)
//   - Arch / Debian: power-profiles-daemon (session bus)
package power

import (
	"fmt"

	"github.com/willeyh-git/sway-power/internal/logger"
)

const (
	// Profile names as defined by power-profiles-daemon.
	ProfilePowerSaver  = "power-saver"
	ProfileBalanced    = "balanced"
	ProfilePerformance = "performance"
)

// Manager communicates with the running power-profiles daemon via D-Bus.
// It auto-detects whether tuned-ppd (Fedora) or power-profiles-daemon
// (Arch, Debian, etc.) is available.
type Manager struct {
	daemon daemon
}

// New creates a Manager, auto-detecting the available daemon.
//
// When debug is true, all D-Bus calls and signals are logged to stderr.
func New(debug bool) (*Manager, error) {
	lg := logger.New(debug, "[power] ")

	// Try tuned-ppd (system bus) first — Fedora default.
	if m, err := newTuned(lg); err == nil {
		return m, nil
	}

	// Fall back to power-profiles-daemon (session bus).
	if m, err := newPPD(lg); err == nil {
		return m, nil
	}

	return nil, fmt.Errorf("no power-profiles daemon found (tuned-ppd or power-profiles-daemon)")
}

// Close releases the D-Bus connection.
func (m *Manager) Close() error {
	return m.daemon.close()
}

// ActiveProfile returns the currently active power profile.
func (m *Manager) ActiveProfile() (string, error) {
	return m.daemon.activeProfile()
}

// SetActiveProfile switches to the given profile.
// Supported values: "power-saver", "balanced", "performance".
func (m *Manager) SetActiveProfile(profile string) error {
	return m.daemon.setActiveProfile(profile)
}

// WatchActiveProfile watches for profile changes and calls fn whenever
// the active profile changes. It returns a function to stop watching.
func (m *Manager) WatchActiveProfile(fn func(profile string)) func() {
	return m.daemon.watchActiveProfile(fn)
}

// daemon is the interface that both tuned-ppd and power-profiles-daemon
// implementations must satisfy.
type daemon interface {
	close() error
	activeProfile() (string, error)
	setActiveProfile(string) error
	watchActiveProfile(func(string)) func()
}
