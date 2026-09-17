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
	inhibitor    *Inhibitor
	monitor      *Monitor
	handler      *Handler
	prefsWatcher *PreferencesWatcher
	log          *logger.Logger
	version      string
}

// New creates a new Daemon.
//
// The daemon always logs to stderr — under systemd that is the journal —
// because the plan requires inhibitor failures to be logged prominently.
// The debug flag only enables chatty per-poll diagnostics. version is the
// build version (embedded via -ldflags); it is logged at startup so a
// stale or just-upgraded daemon is identifiable in the journal.
func New(debug bool, version string) (*Daemon, error) {
	lg := logger.New(true, "[daemon] ")

	// Diagnose the Sway/systemd session environment up front: every
	// missing piece (WAYLAND_DISPLAY, XDG_RUNTIME_DIR, XDG_SESSION_TYPE,
	// swaylock/swaymsg/systemctl on PATH) is logged as a prominent
	// "session:" warning instead of degrading silently.
	checkSession(lg)

	d := &Daemon{log: lg, version: version}

	// Acquire the inhibit lock in the background. If the system bus or
	// logind is unavailable at daemon startup, the Inhibitor retries
	// every 5s; logind handles the lid switch meanwhile (the documented
	// transient double-handler window).
	d.inhibitor = NewInhibitor(lg)

	// Create handler with default action.
	d.handler = NewHandler(action.ActionLock, lg)

	// Create preferences watcher (initializes handler action from prefs).
	d.prefsWatcher = NewPreferencesWatcher(d.handler, lg)

	// Create monitor - wires to handler via callbacks.
	d.monitor = NewMonitor(lg, debug, func(state LidState) {
		switch state {
		case LidClosed:
			d.handler.HandleLidClosed()
		case LidOpen:
			d.handler.HandleLidOpen()
		}
	})

	return d, nil
}

// Run starts the daemon and blocks until SIGTERM/SIGINT.
//
// Shutdown is owned by the caller (main.runDaemon defers it), so it
// happens exactly once.
func (d *Daemon) Run() {
	d.log.Printf("daemon: starting (sway-power %s)", d.version)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigCh)

	<-sigCh
	d.log.Printf("daemon: shutting down")
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
//
// No Validate() here: the stored action is always validated at set
// time (Handler.SetAction / NewHandler), so it is always valid.
func (d *Daemon) GetAction() string {
	if d.handler != nil {
		return string(d.handler.GetAction())
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
