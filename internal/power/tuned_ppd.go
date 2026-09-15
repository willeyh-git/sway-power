package power

import (
	"fmt"
	"os"
	"sync"

	"github.com/godbus/dbus/v5"
)

// tunedPPD implements the daemon interface for Fedora's tuned-ppd.
// It uses the system bus and property-based D-Bus API.
type tunedPPD struct {
	conn  *dbus.Conn
	iface string
	debug bool
	log   func(string, ...any)
}

// newTuned creates a tuned-ppd daemon implementation.
func newTuned(debug bool) (*Manager, error) {
	conn, err := dbus.SystemBus()
	if err != nil {
		return nil, fmt.Errorf("connect to system bus: %w", err)
	}

	d := &tunedPPD{
		conn:  conn,
		iface: "net.hadess.PowerProfiles",
		debug: debug,
		log:   func(msg string, args ...any) { fmt.Fprintf(os.Stderr, "[power] "+msg+"\n", args...) },
	}

	if err := d.ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("tuned-ppd not running: %w", err)
	}

	if debug {
		d.log("connected to tuned-ppd (system bus)")
	}

	return &Manager{daemon: d, debug: debug, log: d.log}, nil
}

func (d *tunedPPD) close() error {
	if d.debug {
		d.log("closing D-Bus connection")
	}
	return d.conn.Close()
}

func (d *tunedPPD) ping() error {
	if d.debug {
		d.log("pinging %s", d.iface)
	}
	obj := d.conn.Object(d.iface, dbus.ObjectPath("/net/hadess/PowerProfiles"))
	var variant dbus.Variant
	err := obj.Call("org.freedesktop.DBus.Properties.Get", 0, d.iface, "ActiveProfile").Store(&variant)
	if err != nil {
		return err
	}
	result := variant.Value().(string)
	if d.debug {
		d.log("ping ok: ActiveProfile=%q", result)
	}
	return nil
}

func (d *tunedPPD) activeProfile() (string, error) {
	if d.debug {
		d.log("→ GetActiveProfile()")
	}
	obj := d.conn.Object(d.iface, dbus.ObjectPath("/net/hadess/PowerProfiles"))
	var variant dbus.Variant
	err := obj.Call("org.freedesktop.DBus.Properties.Get", 0, d.iface, "ActiveProfile").Store(&variant)
	if err != nil {
		return "", fmt.Errorf("get active profile: %w", err)
	}
	result := variant.Value().(string)
	if d.debug {
		d.log("← GetActiveProfile() = %q", result)
	}
	return result, nil
}

func (d *tunedPPD) setActiveProfile(profile string) error {
	if d.debug {
		d.log("→ SetActiveProfile(%q)", profile)
	}
	obj := d.conn.Object(d.iface, dbus.ObjectPath("/net/hadess/PowerProfiles"))
	err := obj.Call("org.freedesktop.DBus.Properties.Set", 0, d.iface, "ActiveProfile", dbus.MakeVariant(profile)).Store()
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

func (d *tunedPPD) watchActiveProfile(debug bool, fn func(string)) func() {
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
		d.log("subscribed to PropertiesChanged signals")
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
				if debug && sig.Name == "org.freedesktop.DBus.Properties.PropertiesChanged" {
					d.log("← signal PropertiesChanged = %v", sig.Body)
				}
				if sig.Name == "org.freedesktop.DBus.Properties.PropertiesChanged" && len(sig.Body) >= 2 {
					ifaceName, _ := sig.Body[0].(string)
					if ifaceName != d.iface && ifaceName != "org.freedesktop.UPower.PowerProfiles" {
						continue
					}
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

	return func() {
		once.Do(func() {
			close(stopCh)
		})
	}
}
