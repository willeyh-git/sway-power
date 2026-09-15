package power

import (
	"fmt"
	"os"
	"sync"

	"github.com/godbus/dbus/v5"
)

// ppd implements the daemon interface for power-profiles-daemon.
// It uses the session bus and method-based D-Bus API.
type ppd struct {
	conn  *dbus.Conn
	iface string
	debug bool
	log   func(string, ...any)
}

// newPPD creates a power-profiles-daemon implementation.
func newPPD(debug bool) (*Manager, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("connect to session bus: %w", err)
	}

	d := &ppd{
		conn:  conn,
		iface: "org.freedesktop.PowerProfiles",
		debug: debug,
		log:   func(msg string, args ...any) { fmt.Fprintf(os.Stderr, "[power] "+msg+"\n", args...) },
	}

	if err := d.ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("power-profiles-daemon not running: %w", err)
	}

	if debug {
		d.log("connected to power-profiles-daemon (session bus)")
	}

	return &Manager{daemon: d, debug: debug, log: d.log}, nil
}

func (d *ppd) close() error {
	if d.debug {
		d.log("closing D-Bus connection")
	}
	return d.conn.Close()
}

func (d *ppd) ping() error {
	if d.debug {
		d.log("pinging %s", d.iface)
	}
	obj := d.conn.Object(d.iface, dbus.ObjectPath("/org/freedesktop/PowerProfiles"))
	var result string
	err := obj.Call(d.iface+".GetActiveProfile", 0).Store(&result)
	if err != nil {
		return err
	}
	if d.debug {
		d.log("ping ok: ActiveProfile=%q", result)
	}
	return nil
}

func (d *ppd) activeProfile() (string, error) {
	if d.debug {
		d.log("→ GetActiveProfile()")
	}
	obj := d.conn.Object(d.iface, dbus.ObjectPath("/org/freedesktop/PowerProfiles"))
	var result string
	err := obj.Call(d.iface+".GetActiveProfile", 0).Store(&result)
	if err != nil {
		return "", fmt.Errorf("get active profile: %w", err)
	}
	if d.debug {
		d.log("← GetActiveProfile() = %q", result)
	}
	return result, nil
}

func (d *ppd) setActiveProfile(profile string) error {
	if d.debug {
		d.log("→ SetActiveProfile(%q)", profile)
	}
	obj := d.conn.Object(d.iface, dbus.ObjectPath("/org/freedesktop/PowerProfiles"))
	err := obj.Call(d.iface+".SetActiveProfile", 0, profile).Store()
	if err != nil {
		if d.debug {
			d.log("← SetActiveProfile(%q) = ERROR: %v", profile, err)
		}
		return fmt.Errorf("set active profile %q: %w", profile, err)
	}
	if d.debug {
		d.log("← SetActiveProfile(%q) = ok", profile)
	}
	return nil
}

func (d *ppd) watchActiveProfile(debug bool, fn func(string)) func() {
	stopCh := make(chan struct{})
	once := sync.Once{}

	sigCh := make(chan *dbus.Signal, 16)
	d.conn.Signal(sigCh)

	match := dbus.WithMatchSender(d.iface)
	if err := d.conn.AddMatchSignal(match); err != nil {
		d.conn.RemoveSignal(sigCh)
		if debug {
			d.log("failed to add signal match: %v", err)
		}
		return func() {}
	}

	if debug {
		d.log("subscribed to ActiveProfileChanged signals")
	}

	go func() {
		defer func() {
			once.Do(func() {
				d.conn.RemoveMatchSignal(match)
				d.conn.RemoveSignal(sigCh)
				close(sigCh)
			})
		}()

		for {
			select {
			case <-stopCh:
				if debug {
					d.log("stopping signal watcher")
				}
				return
			case sig, ok := <-sigCh:
				if !ok {
					return
				}
				if debug && sig.Name == d.iface+".ActiveProfileChanged" {
					d.log("← signal ActiveProfileChanged = %v", sig.Body)
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
