// Package power provides a client for power-profiles-daemon via D-Bus.
// It supports the three standard profiles: power-saver, balanced, performance.
//
// On Fedora the service is tuned-ppd (net.hadess.PowerProfiles) on the
// system bus. On Arch / other distros it's power-profiles-daemon on the
// session bus. This package tries both automatically.
package power

import (
	"fmt"
	"os"

	"github.com/godbus/dbus/v5"
)

const (
	// Profile names as defined by power-profiles-daemon.
	ProfilePowerSaver  = "power-saver"
	ProfileBalanced    = "balanced"
	ProfilePerformance = "performance"

	// power-profiles-daemon (session bus — Arch, Debian, etc.)
	ppdService   = "org.freedesktop.PowerProfiles"
	ppdPath      = "/org/freedesktop/PowerProfiles"
	ppdInterface = "org.freedesktop.PowerProfiles"

	// tuned-ppd (system bus — Fedora)
	tunedService   = "net.hadess.PowerProfiles"
	tunedPath      = "/net/hadess/PowerProfiles"
	tunedInterface = "net.hadess.PowerProfiles"
)

// Manager communicates with the running power-profiles daemon via D-Bus.
type Manager struct {
	conn    *dbus.Conn
	service string
	path    string
	iface   string
	kind    string // "tuned" or "ppd"
	debug   bool
	log     func(string, ...any)
}

// New creates a Manager, auto-detecting whether tuned-ppd (system bus) or
// power-profiles-daemon (session bus) is available.
//
// When debug is true, all D-Bus calls and signals are logged to stderr.
func New(debug bool) (*Manager, error) {
	// Try tuned-ppd (system bus) first — Fedora default.
	if m, err := newTuned(debug); err == nil {
		return m, nil
	}

	// Fall back to power-profiles-daemon (session bus).
	if m, err := newPPD(debug); err == nil {
		return m, nil
	}

	return nil, fmt.Errorf("no power-profiles daemon found (tuned-ppd or power-profiles-daemon)")
}

// newTuned connects to Fedora's tuned-ppd on the system bus.
func newTuned(debug bool) (*Manager, error) {
	conn, err := dbus.SystemBus()
	if err != nil {
		return nil, fmt.Errorf("connect to system bus: %w", err)
	}

	m := &Manager{
		conn:    conn,
		service: tunedService,
		path:    tunedPath,
		iface:   tunedInterface,
		kind:    "tuned",
		debug:   debug,
		log:     func(msg string, args ...any) { fmt.Fprintf(os.Stderr, "[power] "+msg+"\n", args...) },
	}

	if err := m.ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("tuned-ppd not running: %w", err)
	}

	if debug {
		m.log("connected to tuned-ppd (system bus)")
	}

	return m, nil
}

// newPPD connects to power-profiles-daemon on the session bus.
func newPPD(debug bool) (*Manager, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("connect to session bus: %w", err)
	}

	m := &Manager{
		conn:    conn,
		service: ppdService,
		path:    ppdPath,
		iface:   ppdInterface,
		kind:    "ppd",
		debug:   debug,
		log:     func(msg string, args ...any) { fmt.Fprintf(os.Stderr, "[power] "+msg+"\n", args...) },
	}

	if err := m.ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("power-profiles-daemon not running: %w", err)
	}

	if debug {
		m.log("connected to power-profiles-daemon (session bus)")
	}

	return m, nil
}

// Close releases the D-Bus connection.
func (m *Manager) Close() error {
	if m.debug {
		m.log("closing D-Bus connection")
	}
	return m.conn.Close()
}

// ping checks that the daemon is reachable.
func (m *Manager) ping() error {
	obj := m.conn.Object(m.service, dbus.ObjectPath(m.path))
	if m.debug {
		m.log("pinging %s %s", m.service, m.path)
	}
	var result string
	var err error

	if m.kind == "tuned" {
		// tuned-ppd: property-based API.
		var variant dbus.Variant
		err = obj.Call("org.freedesktop.DBus.Properties.Get", 0, m.iface, "ActiveProfile").Store(&variant)
		if err == nil {
			result = variant.Value().(string)
		}
	} else {
		// power-profiles-daemon: method-based API.
		err = obj.Call(m.iface+".GetActiveProfile", 0).Store(&result)
	}

	if err != nil {
		if m.debug {
			m.log("ping failed: %v", err)
		}
		return err
	}
	if m.debug {
		m.log("ping ok: ActiveProfile=%q", result)
	}
	return nil
}

// ActiveProfile returns the currently active power profile.
func (m *Manager) ActiveProfile() (string, error) {
	obj := m.conn.Object(m.service, dbus.ObjectPath(m.path))
	if m.debug {
		m.log("→ GetActiveProfile()")
	}

	var result string
	var err error

	if m.kind == "tuned" {
		// tuned-ppd: property-based API.
		var variant dbus.Variant
		err = obj.Call("org.freedesktop.DBus.Properties.Get", 0, m.iface, "ActiveProfile").Store(&variant)
		if err == nil {
			result = variant.Value().(string)
		}
	} else {
		// power-profiles-daemon: method-based API.
		err = obj.Call(m.iface+".GetActiveProfile", 0).Store(&result)
	}

	if err != nil {
		return "", fmt.Errorf("get active profile: %w", err)
	}
	if m.debug {
		m.log("← GetActiveProfile() = %q", result)
	}
	return result, nil
}

// SetActiveProfile switches to the given profile.
// Supported values: "power-saver", "balanced", "performance".
func (m *Manager) SetActiveProfile(profile string) error {
	obj := m.conn.Object(m.service, dbus.ObjectPath(m.path))
	if m.debug {
		m.log("→ SetActiveProfile(%q)", profile)
	}

	var err error

	if m.kind == "tuned" {
		// tuned-ppd: property-based API.
		err = obj.Call("org.freedesktop.DBus.Properties.Set", 0, m.iface, "ActiveProfile", dbus.MakeVariant(profile)).Store()
	} else {
		// power-profiles-daemon: method-based API.
		err = obj.Call(m.iface+".SetActiveProfile", 0, profile).Store()
	}

	if err != nil {
		if m.debug {
			m.log("← SetActiveProfile(%q) = ERROR: %v", profile, err)
		}
		return fmt.Errorf("set active profile %q: %w", profile, err)
	}
	if m.debug {
		m.log("← SetActiveProfile(%q) = ok", profile)
	}
	return nil
}

// WatchActiveProfile watches for profile changes and calls fn whenever
// the active profile changes. It returns a function to stop watching.
func (m *Manager) WatchActiveProfile(debug bool, fn func(profile string)) func() {
	stopCh := make(chan struct{})

	// Subscribe to all signals on the bus.
	sigCh := make(chan *dbus.Signal, 16)
	m.conn.Signal(sigCh)

	// Add match rule for our signal.
	match := dbus.WithMatchSender(m.service)
	if err := m.conn.AddMatchSignal(match); err != nil {
		m.conn.RemoveSignal(sigCh)
		if debug {
			m.log("failed to add signal match: %v", err)
		}
		return func() {}
	}

	if debug {
		m.log("subscribed to PropertiesChanged signals")
	}

	go func() {
		defer func() {
			m.conn.RemoveMatchSignal(match)
			m.conn.RemoveSignal(sigCh)
			close(sigCh)
		}()

		for {
			select {
			case <-stopCh:
				if debug {
					m.log("stopping signal watcher")
				}
				return
			case sig, ok := <-sigCh:
				if !ok {
					return
				}
				if debug && sig.Name == "org.freedesktop.DBus.Properties.PropertiesChanged" {
					m.log("← signal PropertiesChanged = %v", sig.Body)
				}
				// Both tuned-ppd and PPD emit PropertiesChanged.
				if sig.Name == "org.freedesktop.DBus.Properties.PropertiesChanged" && len(sig.Body) >= 2 {
					// Body[0] = interface name, Body[1] = changed properties.
					ifaceName, _ := sig.Body[0].(string)
					if ifaceName != m.iface && ifaceName != "org.freedesktop.UPower.PowerProfiles" {
						continue
					}
					// Body[1] can be map[string]dbus.Variant or []dbus.Variant.
					switch changed := sig.Body[1].(type) {
					case map[string]dbus.Variant:
						if v, ok := changed["ActiveProfile"]; ok {
							if profile, ok := v.Value().(string); ok {
								fn(profile)
							}
						}
					case []dbus.Variant:
						for _, v := range changed {
							if s, ok := v.Value().(string); ok {
								fn(s)
								break
							}
						}
					}
				}
			}
		}
	}()

	return func() { close(stopCh) }
}
