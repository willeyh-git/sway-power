package daemon

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/willeyh-git/sway-power/internal/lid/action"
	"github.com/willeyh-git/sway-power/internal/logger"
)

// Daemon is the background lid handler. It runs until interrupted.
type Daemon struct {
	inhibitor   *Inhibitor
	monitor     *Monitor
	handler     *Handler
	prefsWatcher *PreferencesWatcher
	log         *logger.Logger
}

// New creates a new Daemon.
func New(debug bool) (*Daemon, error) {
	lg := logger.New(debug, "[daemon] ")

	d := &Daemon{log: lg}

	// Acquire the inhibit lock in the background. If the session bus or
	// logind is unavailable at daemon startup, the Inhibitor retries
	// every 5s; logind handles the lid switch meanwhile (the documented
	// transient double-handler window).
	d.inhibitor = NewInhibitor(lg)

	// Create handler with default action.
	d.handler = NewHandler(action.ActionLock, lg)

	// Create preferences watcher (initializes handler action from prefs).
	d.prefsWatcher = NewPreferencesWatcher(d.handler, lg)

	// Create monitor - wires to handler via callbacks.
	d.monitor = NewMonitor(lg, func(state LidState) {
		switch state {
		case LidClosed:
			d.handler.HandleLidClosed()
		case LidOpen:
			d.handler.HandleLidOpen()
		}
	})

	return d, nil
}

// Run starts the daemon and blocks until shutdown.
func (d *Daemon) Run() {
	d.log.Printf("daemon: starting")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	<-sigCh
	d.log.Printf("daemon: shutting down")
	d.Shutdown()
}

// Shutdown stops all components.
func (d *Daemon) Shutdown() {
	if d.monitor != nil {
		d.monitor.Stop()
	}
	if d.prefsWatcher != nil {
		d.prefsWatcher.Stop()
	}
	if d.inhibitor != nil {
		d.inhibitor.Release()
	}
}

// GetAction returns the current action for status reporting.
func (d *Daemon) GetAction() string {
	if d.handler != nil {
		a := d.handler.GetAction()
		if a.Validate() == nil {
			return string(a)
		}
	}
	return "unknown"
}

// GetInhibitorState returns the inhibitor state for status reporting.
func (d *Daemon) GetInhibitorState() string {
	if d.inhibitor != nil {
		return string(d.inhibitor.State())
	}
	return "none"
}
