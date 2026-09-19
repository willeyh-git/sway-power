//go:build integration

package daemon

import (
	"bufio"
	"fmt"
	"os"
	"testing"
	"time"
)

// TestRealLidEventReachesHandler is kernel ↔ evdev ↔ monitor ↔ handler
// with no fake in the chain, plus real swaymsg when an external display
// is connected.
//
// Opt-in and interactive: requires SWAY_POWER_REAL_TESTS=1, a lid switch
// discoverable on this machine, and a person at the keyboard and the
// lid (ENTER confirm on top of the env var — defense in depth against
// CI or cron). The child daemon runs with action "nothing" — the only
// action whose effect is observable from its own log without side
// effects.
func TestRealLidEventReachesHandler(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: skipped by -short")
	}
	requireReal(t)

	// Prerequisite: a lid switch discoverable on this machine (the same
	// check the Monitor performs on startup, against the real trees).
	probing := &recordingLogger{}
	eventDevice, stateFile := findLidSources(probing, "/", "/dev")
	if eventDevice == "" && stateFile == "" {
		t.Skip("no lid switch discoverable on this machine")
	}
	t.Logf("lid sources: evdev=%q state=%q", eventDevice, stateFile)

	baseline := swayPowerLockCount(t)

	// Child daemon, action "nothing", isolated config.
	d := runDaemon(t)
	waitFor(t, 15*time.Second, "child daemon to hold the inhibit lock", func() bool {
		return swayPowerLockCount(t) == baseline+1
	})

	fmt.Fprintln(os.Stderr, `\nNow: press ENTER, then produce one full lid cycle and finish with the lid open — if the lid is OPEN: close it, then open it; if it is already CLOSED: open it, close it, then open it.`)
	if !confirmKeystroke(t) {
		return
	}
	fmt.Fprintln(os.Stderr, "Waiting for a lid close, then a lid open ...")

	// A real evdev SW_LID event (or the 500 ms poll backstop — both are
	// legitimate sources; this asserts correctness, not source) reached
	// the handler. The monitor's initial-state callback fires once at
	// startup and its dedup suppresses repeats, so every occurrence of
	// a handler line is a distinct real transition: the counts below
	// are the race-free "happened after" check.
	const closeLine = `handler: executing action "nothing" on lid close`
	const openLine = "handler: handling lid open"
	// At most one open line exists at this point: the startup one.
	openAt := d.logCount(openLine)
	waitFor(t, 120*time.Second, "a lid close to be handled", func() bool {
		return d.logCount(closeLine) > 0
	})
	// The transition back: one more open line than at prompt time.
	waitFor(t, 120*time.Second, "a lid open to be handled after the close", func() bool {
		return d.logCount(openLine) > openAt
	})

	// Afterwards the daemon must be healthy: alive, lock still held.
	if !d.alive() {
		t.Fatalf("child daemon died during the lid cycle: %v", d.exitErr())
	}
	if got := swayPowerLockCount(t); got != baseline+1 {
		t.Fatalf("inhibit lock count after lid cycle: %d, want %d", got, baseline+1)
	}
	// Teardown (SIGTERM → Shutdown → lock release) runs via runDaemon.
}

// confirmKeystroke waits for ENTER on stdin before the test touches the
// user's hardware; a closed/empty stdin (unattended run) fails rather
// than proceeding.
func confirmKeystroke(t *testing.T) bool {
	t.Helper()
	if _, err := bufio.NewReader(os.Stdin).ReadString('\n'); err != nil {
		t.Fatalf("waiting for ENTER: %v (no tty? this test is interactive)", err)
	}
	return true
}
