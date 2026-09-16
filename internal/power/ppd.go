// Package power manages power profiles through a power-profiles-daemon compatible daemon.
package power

import (
	"fmt"
	"sync"

	"github.com/godbus/dbus/v5"

	"github.com/willeyh-git/sway-power/internal/logger"
)

// ppd implements the daemon interface for power-profiles-daemon.
// It uses the session bus and method-based D-Bus API.
type ppd struct {
	conn  *dbus.Conn
	iface string
	log   *logger.Logger
}

// newPPD creates a power-profiles-daemon implementation.
func newPPD(lg *logger.Logger) (*Manager, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("connect to session bus: %w", err)
	}

	d := &ppd{
		conn:  conn,
		iface: "org.freedesktop.PowerProfiles",
		log:   lg,
	}

	if err := d.ping(); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			d.log.Printf("failed to close D-Bus connection: %v", closeErr)
		}
		return nil, fmt.Errorf("power-profiles-daemon not running: %w", err)
	}

	d.log.Printf("connected to power-profiles-daemon (session bus)")

	return &Manager{daemon: d}, nil
}

func (d *ppd) close() error {
	d.log.Printf("closing D-Bus connection")
	return d.conn.Close()
}

func (d *ppd) ping() error {
	d.log.Printf("pinging %s", d.iface)
	obj := d.conn.Object(d.iface, dbus.ObjectPath("/org/freedesktop/PowerProfiles"))
	var result string
	err := obj.Call(d.iface+".GetActiveProfile", 0).Store(&result)
	if err != nil {
		return err
	}
	d.log.Printf("ping ok: ActiveProfile=%q", result)
	return nil
}

func (d *ppd) activeProfile() (string, error) {
	d.log.Printf("→ GetActiveProfile()")
	obj := d.conn.Object(d.iface, dbus.ObjectPath("/org/freedesktop/PowerProfiles"))
	var result string
	err := obj.Call(d.iface+".GetActiveProfile", 0).Store(&result)
	if err != nil {
		return "", fmt.Errorf("get active profile: %w", err)
	}
	d.log.Printf("← GetActiveProfile() = %q", result)
	return result, nil
}

func (d *ppd) setActiveProfile(profile string) error {
	d.log.Printf("→ SetActiveProfile(%q)", profile)
	obj := d.conn.Object(d.iface, dbus.ObjectPath("/org/freedesktop/PowerProfiles"))
	err := obj.Call(d.iface+".SetActiveProfile", 0, profile).Store()
	if err != nil {
		d.log.Printf("← SetActiveProfile(%q) = ERROR: %v", profile, err)
		return fmt.Errorf("set active profile %q: %w", profile, err)
	}
	d.log.Printf("← SetActiveProfile(%q) = ok", profile)
	return nil
}

func (d *ppd) watchActiveProfile(fn func(string)) func() {
	stopCh := make(chan struct{})
	once := sync.Once{}

	sigCh := make(chan *dbus.Signal, 16)
	d.conn.Signal(sigCh)

	match := dbus.WithMatchSender(d.iface)
	if err := d.conn.AddMatchSignal(match); err != nil {
		d.conn.RemoveSignal(sigCh)
		d.log.Printf("failed to add signal match: %v", err)
		return func() {}
	}

	d.log.Printf("subscribed to ActiveProfileChanged signals")

	go func() {
		defer func() {
			once.Do(func() {
				if err := d.conn.RemoveMatchSignal(match); err != nil {
					d.log.Printf("failed to remove signal match: %v", err)
				}
				d.conn.RemoveSignal(sigCh)
				close(sigCh)
			})
		}()

		for {
			select {
			case <-stopCh:
				d.log.Printf("stopping signal watcher")
				return
			case sig, ok := <-sigCh:
				if !ok {
					return
				}
				if sig.Name == d.iface+".ActiveProfileChanged" {
					d.log.Printf("← signal ActiveProfileChanged = %v", sig.Body)
				}
				if sig.Name == d.iface+".ActiveProfileChanged" && len(sig.Body) > 0 {
					if profile, ok := sig.Body[0].(string); ok {
						fn(profile)
					}
				}
			}
		}
	}()

	return func() {
		once.Do(func() {
			close(stopCh)
		})
	}
}
