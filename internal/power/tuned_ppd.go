package power

import (
	"fmt"
	"sync"

	"github.com/godbus/dbus/v5"

	"github.com/willeyh-git/sway-power/internal/logger"
)

// tunedPPD implements the daemon interface for Fedora's tuned-ppd.
// It uses the system bus and property-based D-Bus API.
type tunedPPD struct {
	conn  *dbus.Conn
	iface string
	log   *logger.Logger
}

// newTuned creates a tuned-ppd daemon implementation.
func newTuned(lg *logger.Logger) (*Manager, error) {
	conn, err := dbus.SystemBus()
	if err != nil {
		return nil, fmt.Errorf("connect to system bus: %w", err)
	}

	d := &tunedPPD{
		conn:  conn,
		iface: "net.hadess.PowerProfiles",
		log:   lg,
	}

	if err := d.ping(); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			d.log.Printf("failed to close D-Bus connection: %v", closeErr)
		}
		return nil, fmt.Errorf("tuned-ppd not running: %w", err)
	}

	d.log.Printf("connected to tuned-ppd (system bus)")

	return &Manager{daemon: d}, nil
}

func (d *tunedPPD) close() error {
	d.log.Printf("closing D-Bus connection")
	return d.conn.Close()
}

func (d *tunedPPD) ping() error {
	d.log.Printf("pinging %s", d.iface)
	obj := d.conn.Object(d.iface, dbus.ObjectPath("/net/hadess/PowerProfiles"))
	var variant dbus.Variant
	err := obj.Call("org.freedesktop.DBus.Properties.Get", 0, d.iface, "ActiveProfile").Store(&variant)
	if err != nil {
		return err
	}
	result := variant.Value().(string)
	d.log.Printf("ping ok: ActiveProfile=%q", result)
	return nil
}

func (d *tunedPPD) activeProfile() (string, error) {
	d.log.Printf("→ GetActiveProfile()")
	obj := d.conn.Object(d.iface, dbus.ObjectPath("/net/hadess/PowerProfiles"))
	var variant dbus.Variant
	err := obj.Call("org.freedesktop.DBus.Properties.Get", 0, d.iface, "ActiveProfile").Store(&variant)
	if err != nil {
		return "", fmt.Errorf("get active profile: %w", err)
	}
	result := variant.Value().(string)
	d.log.Printf("← GetActiveProfile() = %q", result)
	return result, nil
}

func (d *tunedPPD) setActiveProfile(profile string) error {
	d.log.Printf("→ SetActiveProfile(%q)", profile)
	obj := d.conn.Object(d.iface, dbus.ObjectPath("/net/hadess/PowerProfiles"))
	err := obj.Call("org.freedesktop.DBus.Properties.Set", 0, d.iface, "ActiveProfile", dbus.MakeVariant(profile)).Store()
	if err != nil {
		d.log.Printf("← SetActiveProfile(%q) = ERROR: %v", profile, err)
		return fmt.Errorf("set active profile %q: %w", profile, err)
	}
	d.log.Printf("← SetActiveProfile(%q) = ok", profile)
	return nil
}

func (d *tunedPPD) watchActiveProfile(fn func(string)) func() {
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

	d.log.Printf("subscribed to PropertiesChanged signals")

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
				if sig.Name == "org.freedesktop.DBus.Properties.PropertiesChanged" {
					d.log.Printf("← signal PropertiesChanged = %v", sig.Body)
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
