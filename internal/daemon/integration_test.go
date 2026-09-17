package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/willeyh-git/sway-power/internal/lid/action"
)

// This file holds the integration tests for the failure modes listed in
// TODO.md (#7). Each test wires the real daemon components together the
// way Daemon.New does — Inhibitor, Monitor, Handler and
// PreferencesWatcher — and only substitutes the seams the production
// code already exposes for tests:
//
//   - a mock D-Bus connection for the Inhibitor (logind disappearing),
//   - a fake /sys+/proc lid tree for the Monitor (real 500 ms polling),
//   - a fake sway for the actions (get_outputs / output enable/disable),
//   - a temp preferences.json for the PreferencesWatcher (real 1 s poll).
//
// Everything in between — the inhibitor state machine, lid state
// delivery, display ownership, prefs parsing — is the real code path.

// swayFake is a stateful fake of the sway compositor. get_outputs
// reports the internal eDP-1 (reflecting its current enabled state) and,
// when present, the external HDMI-A-1. `output eDP-1 enable|disable`
// updates that state. Every call is also recorded in the mockExec.
type swayFake struct {
	mu   sync.Mutex
	edp  bool // eDP-1 enabled
	hdmi bool // HDMI-A-1 present (and enabled)
}

func newSwayFake(t *testing.T, mock *mockExec, edp, hdmi bool) *swayFake {
	t.Helper()
	f := &swayFake{edp: edp, hdmi: hdmi}
	prev := action.SwaymsgCmd
	t.Cleanup(func() { action.SwaymsgCmd = prev })
	action.SwaymsgCmd = func(args ...string) *exec.Cmd {
		return f.cmd(mock, args...)
	}
	return f
}

func (f *swayFake) cmd(mock *mockExec, args ...string) *exec.Cmd {
	mock.exec("swaymsg", args...)
	switch {
	case len(args) == 3 && args[0] == "output" && args[1] == "eDP-1" &&
		(args[2] == "enable" || args[2] == "disable"):
		f.mu.Lock()
		f.edp = args[2] == "enable"
		f.mu.Unlock()
		return exec.Command("true")
	case len(args) == 3 && args[0] == "-t" && args[1] == "json" && args[2] == "get_outputs":
		f.mu.Lock()
		defer f.mu.Unlock()
		edp := "false"
		if f.edp {
			edp = "true"
		}
		var b strings.Builder
		fmt.Fprintf(&b, `[{"name":"eDP-1","interface":"eDP-1","enabled":%s}`, edp)
		if f.hdmi {
			b.WriteString(`,{"name":"HDMI-A-1","interface":"HDMI-A-1","enabled":true}`)
		}
		b.WriteString(`]`)
		return exec.Command("echo", b.String())
	}
	return exec.Command("true")
}

// setEDP simulates the internal panel being toggled outside of
// sway-power's own commands (e.g. the user running `swaymsg output
// eDP-1 off`).
func (f *swayFake) setEDP(enabled bool) {
	f.mu.Lock()
	f.edp = enabled
	f.mu.Unlock()
}

// setHDMI simulates the external monitor appearing/disappearing (a
// connect event, not an enable/disable command).
func (f *swayFake) setHDMI(present bool) {
	f.mu.Lock()
	f.hdmi = present
	f.mu.Unlock()
}

func (f *swayFake) edpEnabled() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.edp
}

// integrationDaemon is a near-complete daemon for tests: the real
// Inhibitor, Monitor, Handler and PreferencesWatcher, wired exactly like
// Daemon.New (monitor callback → HandleLidClosed/HandleLidOpen, prefs
// watcher → SetAction), with fakes at the four seams above.
type integrationDaemon struct {
	mock      *mockExec
	fake      *swayFake
	monitor   *Monitor
	handler   *Handler
	inhibitor *Inhibitor
	bus       *busFactory
	pw        *PreferencesWatcher
	rec       *lidRecorder
	stateFile string
	prefsPath string
}

type integrationOpts struct {
	// lidClosedAtStart makes the state file report "closed" before the
	// monitor starts, so the initial state callback is a lid-close.
	lidClosedAtStart bool
	// initialAction is set on the handler and written to the temp
	// preferences.json.
	initialAction action.Action
	// edpEnabled / hdmiPresent are the initial sway display states.
	edpEnabled  bool
	hdmiPresent bool
}

func startIntegrationDaemon(t *testing.T, opts integrationOpts) *integrationDaemon {
	t.Helper()
	log := &testLogger{t}
	_, mock := setupMockActions(t)

	d := &integrationDaemon{
		mock: mock,
		bus:  &busFactory{},
		rec:  &lidRecorder{},
	}
	d.fake = newSwayFake(t, mock, opts.edpEnabled, opts.hdmiPresent)

	// Handler (same default-validation path as New).
	d.handler = NewHandler(opts.initialAction, log)

	// Inhibitor on the mock bus (fast retry).
	d.inhibitor = newTestInhibitor(t, d.bus)

	// Monitor on the fake lid tree: state file only, so the real 500 ms
	// poll path drives every transition.
	sysRoot, _ := setupLidTree(t, false, true, false)
	d.stateFile = filepath.Join(sysRoot, "proc/acpi/button/lid/LID0/state")
	if opts.lidClosedAtStart {
		if err := os.WriteFile(d.stateFile, []byte("Lid switch is closed\n"), 0644); err != nil {
			t.Fatalf("write closed state: %v", err)
		}
	}
	// The handler runs BEFORE the recorder advances, so closeLid/openLid
	// return only after the handler finished the transition.
	d.monitor = newMonitor(log, false, func(s LidState) {
		if s == LidClosed {
			d.handler.HandleLidClosed()
		} else {
			d.handler.HandleLidOpen()
		}
		d.rec.add(s)
	}, sysRoot, t.TempDir())
	if d.monitor == nil {
		t.Fatal("expected monitor")
	}
	t.Cleanup(func() { d.monitor.Stop() })

	// Preferences watcher on a temp preferences.json.
	d.prefsPath = filepath.Join(t.TempDir(), "preferences.json")
	if err := os.WriteFile(d.prefsPath, []byte(fmt.Sprintf(`{"lid_close": "%s"}`, opts.initialAction)), 0644); err != nil {
		t.Fatalf("write prefs: %v", err)
	}
	d.pw = &PreferencesWatcher{
		prefsPath: d.prefsPath,
		handler:   d.handler,
		log:       log,
		stopCh:    make(chan struct{}),
		done:      make(chan struct{}),
	}
	go d.pw.watch()
	t.Cleanup(d.pw.Stop)

	return d
}

// closeLid flips the state file to closed and blocks until the monitor's
// 500 ms poll picked it up AND the handler finished processing the
// lid-close.
func (d *integrationDaemon) closeLid(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(d.stateFile, []byte("Lid switch is closed\n"), 0644); err != nil {
		t.Fatalf("write state: %v", err)
	}
	d.expectLidState(t, LidClosed)
}

// openLid is closeLid for the open state.
func (d *integrationDaemon) openLid(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(d.stateFile, []byte("Lid switch is open\n"), 0644); err != nil {
		t.Fatalf("write state: %v", err)
	}
	d.expectLidState(t, LidOpen)
}

// expectLidState blocks until the monitor has delivered (and the handler
// finished processing) a transition to s.
func (d *integrationDaemon) expectLidState(t *testing.T, s LidState) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if states := d.rec.snapshot(); len(states) > 0 && states[len(states)-1] == s {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for lid state %s (got %v)", s, d.rec.snapshot())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// setAction changes the action via preferences.json — the same path the
// GUI uses — and blocks until the watcher swapped it on the handler.
func (d *integrationDaemon) setAction(t *testing.T, a action.Action) {
	t.Helper()
	if err := os.WriteFile(d.prefsPath, []byte(fmt.Sprintf(`{"lid_close": "%s"}`, a)), 0644); err != nil {
		t.Fatalf("write prefs: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		if d.handler.GetAction() == a {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for watcher to swap action to %q (handler has %q)", a, d.handler.GetAction())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// countCalls counts recorded mockExec calls with exactly the given
// name/args.
func countCalls(m *mockExec, name string, args ...string) int {
	n := 0
	for _, c := range m.getCalls() {
		if c.name != name || len(c.args) != len(args) {
			continue
		}
		same := true
		for i := range args {
			if c.args[i] != args[i] {
				same = false
				break
			}
		}
		if same {
			n++
		}
	}
	return n
}

// TestIntegrationLogindDisappearsAndReappears is the failure mode "logind
// disappearing/reappearing while daemon is alive": while holding the
// inhibit lock, logind goes away (bus connection survives) and the
// daemon must keep handling lid events; when logind comes back the
// Inhibitor re-acquires the lock on a fresh connection.
func TestIntegrationLogindDisappearsAndReappears(t *testing.T) {
	d := startIntegrationDaemon(t, integrationOpts{
		initialAction: action.ActionNothing,
		edpEnabled:    true,
		hdmiPresent:   true,
	})

	waitForState(t, d.inhibitor, StateAcquired)
	before := d.bus.count()

	// A normal close/open round trip works.
	d.closeLid(t)
	if !d.mock.called("swaymsg", "output", "eDP-1", "disable") {
		t.Fatalf("expected eDP-1 disable on close, got: %v", d.mock.getCalls())
	}
	d.openLid(t)
	if !d.mock.called("swaymsg", "output", "eDP-1", "enable") {
		t.Fatalf("expected eDP-1 enable on open, got: %v", d.mock.getCalls())
	}

	// logind disappears (restart) while the daemon is alive.
	d.bus.at(0).dropLogind()

	// The daemon must stay fully functional while logind is gone: a
	// close/open still round-trips the display.
	d.closeLid(t)
	if got := countCalls(d.mock, "swaymsg", "output", "eDP-1", "disable"); got != 2 {
		t.Errorf("expected 2nd disable while logind is gone, got %d (calls: %v)", got, d.mock.getCalls())
	}
	d.openLid(t)
	if got := countCalls(d.mock, "swaymsg", "output", "eDP-1", "enable"); got != 2 {
		t.Errorf("expected 2nd enable while logind is gone, got %d (calls: %v)", got, d.mock.getCalls())
	}
	if !d.fake.edpEnabled() {
		t.Errorf("eDP-1 should be enabled after open")
	}

	// logind reappears: the Inhibitor re-acquires the lock on a fresh
	// connection, and the handler still works afterwards.
	expectReacquire(t, d.bus, d.inhibitor, before)
	if got := d.inhibitor.State(); got != StateAcquired {
		t.Errorf("after logind reappears: state %q, want acquired", got)
	}
	if got := d.bus.count(); got != 2 {
		t.Errorf("expected reacquire on a 2nd connection, got %d connections", got)
	}
	if c := d.bus.at(1).calls(); c != 1 {
		t.Errorf("expected 1 Inhibit call on the 2nd connection, got %d", c)
	}

	d.closeLid(t)
	if got := countCalls(d.mock, "swaymsg", "output", "eDP-1", "disable"); got != 3 {
		t.Errorf("expected 3rd disable after re-acquire, got %d", got)
	}
	d.openLid(t)
	if got := countCalls(d.mock, "swaymsg", "output", "eDP-1", "enable"); got != 3 {
		t.Errorf("expected 3rd enable after re-acquire, got %d", got)
	}
}

// TestIntegrationLidClosedAtDaemonStartup is the failure mode "lid already
// closed at daemon startup": the monitor's initial state callback is a
// lid-close, so the current action must run on startup, and the
// ownership it claims is restored on the first lid open.
func TestIntegrationLidClosedAtDaemonStartup(t *testing.T) {
	d := startIntegrationDaemon(t, integrationOpts{
		lidClosedAtStart: true,
		initialAction:    action.ActionNothing,
		edpEnabled:       true,
		hdmiPresent:      true,
	})

	// Wait until the initial "closed" state has been processed.
	d.closeLid(t)

	// The action ran on startup: the internal display was disabled
	// (external monitor connected).
	if !d.mock.called("swaymsg", "output", "eDP-1", "disable") {
		t.Fatalf("expected eDP-1 disabled at startup with lid closed, got: %v", d.mock.getCalls())
	}
	if d.fake.edpEnabled() {
		t.Errorf("eDP-1 should be disabled after startup with lid closed")
	}

	// The first lid open restores exactly what we disabled.
	d.openLid(t)
	if !d.mock.called("swaymsg", "output", "eDP-1", "enable") {
		t.Fatalf("expected eDP-1 enable on first open, got: %v", d.mock.getCalls())
	}
	if !d.fake.edpEnabled() {
		t.Errorf("eDP-1 should be enabled after first open")
	}

	// The inhibitor keeps working in parallel: it acquired (and holds)
	// the lock throughout the test.
	waitForState(t, d.inhibitor, StateAcquired)
}

// TestIntegrationActionChangeWhileLidClosed covers the failure modes
// "action change nothing → lock while lid is closed" and "action change
// nothing → sleep while lid is closed": with the lid closed (eDP-1
// disabled by us) the user swaps the action via preferences.json, and the
// lid open must still restore eDP-1 — while the new action must NOT run
// on open.
func TestIntegrationActionChangeWhileLidClosed(t *testing.T) {
	for _, newAction := range []action.Action{action.ActionLock, action.ActionSleep} {
		t.Run(string(newAction), func(t *testing.T) {
			d := startIntegrationDaemon(t, integrationOpts{
				initialAction: action.ActionNothing,
				edpEnabled:    true,
				hdmiPresent:   true,
			})

			d.closeLid(t)
			if !d.mock.called("swaymsg", "output", "eDP-1", "disable") {
				t.Fatalf("expected eDP-1 disable on close, got: %v", d.mock.getCalls())
			}

			// The user changes the action while the lid stays closed.
			d.setAction(t, newAction)
			if got := d.handler.GetAction(); got != newAction {
				t.Fatalf("action = %q, want %q", got, newAction)
			}

			d.openLid(t)

			// eDP-1 is restored even though the current action no longer
			// knows about it.
			if !d.mock.called("swaymsg", "output", "eDP-1", "enable") {
				t.Errorf("eDP-1 must be restored on open even though the action is now %q, got: %v", newAction, d.mock.getCalls())
			}
			if !d.fake.edpEnabled() {
				t.Errorf("eDP-1 should be enabled after open")
			}

			// The new action must not run on lid open.
			if d.mock.called("swaylock", "-f") {
				t.Errorf("swaylock must not run on lid open, got: %v", d.mock.getCalls())
			}
			if d.mock.called("systemctl", "suspend") {
				t.Errorf("systemctl suspend must not run on lid open, got: %v", d.mock.getCalls())
			}
		})
	}
}

// TestIntegrationManuallyDisabledEDP is the failure mode "manually
// disabled eDP output (must not be re-enabled)": the user turned off the
// internal panel themselves. A lid close with "nothing" must not claim
// it, and a lid open must not re-enable it.
func TestIntegrationManuallyDisabledEDP(t *testing.T) {
	d := startIntegrationDaemon(t, integrationOpts{
		initialAction: action.ActionNothing,
		edpEnabled:    true,
		hdmiPresent:   true,
	})

	// The user manually disables the internal panel (external monitor is
	// still connected).
	d.fake.setEDP(false)

	d.closeLid(t)
	if d.mock.called("swaymsg", "output", "eDP-1", "disable") {
		t.Fatalf("must not disable a user-disabled output, got: %v", d.mock.getCalls())
	}

	d.openLid(t)
	if d.mock.called("swaymsg", "output", "eDP-1", "enable") {
		t.Fatalf("must not re-enable a user-disabled output, got: %v", d.mock.getCalls())
	}
	if d.fake.edpEnabled() {
		t.Errorf("user-disabled eDP-1 must stay disabled")
	}

	// Sanity: the machinery still works once the user re-enables the
	// panel themselves — a later close/open round-trips it.
	d.fake.setEDP(true)
	d.closeLid(t)
	if !d.mock.called("swaymsg", "output", "eDP-1", "disable") {
		t.Fatalf("expected eDP-1 disable once it is enabled again, got: %v", d.mock.getCalls())
	}
	d.openLid(t)
	if !d.fake.edpEnabled() {
		t.Errorf("eDP-1 should be restored after the round trip")
	}
}

// TestIntegrationExternalMonitorWhileLidClosed is the failure mode
// "external monitor appearing/disappearing while lid is closed": while
// the lid is closed (eDP-1 owned by sway-power), the external monitor
// plugs out and back in. Nothing may break: on open we still restore
// exactly eDP-1, and ownership bookkeeping stays correct afterwards.
func TestIntegrationExternalMonitorWhileLidClosed(t *testing.T) {
	d := startIntegrationDaemon(t, integrationOpts{
		initialAction: action.ActionNothing,
		edpEnabled:    true,
		hdmiPresent:   true,
	})

	// Close: eDP-1 disabled while the external monitor is connected.
	d.closeLid(t)
	if !d.mock.called("swaymsg", "output", "eDP-1", "disable") {
		t.Fatalf("expected eDP-1 disable on close, got: %v", d.mock.getCalls())
	}

	// The external monitor disappears while the lid is closed.
	d.fake.setHDMI(false)

	// Lid open: eDP-1 is still restored — restoration is by recorded
	// name, not by re-deriving from the (changed) output list.
	d.openLid(t)
	if !d.mock.called("swaymsg", "output", "eDP-1", "enable") {
		t.Fatalf("expected eDP-1 enable on open, got: %v", d.mock.getCalls())
	}
	if !d.fake.edpEnabled() {
		t.Errorf("eDP-1 should be enabled after open")
	}

	// The external monitor appears again; a close/open round trip still
	// works and leaves the display on.
	d.fake.setHDMI(true)
	d.closeLid(t)
	if got := countCalls(d.mock, "swaymsg", "output", "eDP-1", "disable"); got != 2 {
		t.Errorf("expected 2nd disable after monitor reappears, got %d (calls: %v)", got, d.mock.getCalls())
	}
	d.openLid(t)
	if got := countCalls(d.mock, "swaymsg", "output", "eDP-1", "enable"); got != 2 {
		t.Errorf("expected 2nd enable, got %d (calls: %v)", got, d.mock.getCalls())
	}
	if !d.fake.edpEnabled() {
		t.Errorf("eDP-1 should be enabled at the end")
	}
}
