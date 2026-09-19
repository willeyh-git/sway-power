//go:build integration

package daemon

import (
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"
)

// TestRealInhibitorAcquireAndRecover is the core correctness property of
// the architecture, on the real transport: real daemon.NewInhibitor,
// real system bus, real logind, real SCM_RIGHTS fd transfer.
//
// Safe tier: acquires a genuine "handle-lid-switch" block lock on the
// user's session for the duration of the test (a few seconds) and
// releases it in teardown. The recovery subtest is opt-in
// (SWAY_POWER_REAL_TESTS=1 + root) because it restarts systemd-logind.
func TestRealInhibitorAcquireAndRecover(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: skipped by -short")
	}
	requireSystemBus(t)

	// Baseline before this test holds anything: may include the
	// user's own running sway-power service.
	baseline := swayPowerLockCount(t)

	t.Run("acquire and release", func(t *testing.T) {
		inh := NewInhibitor(&recordingLogger{})
		t.Cleanup(inh.Release)

		// Real retry interval is 5 s; 15 s allows two cycles.
		waitFor(t, 15*time.Second, "inhibitor to reach acquired", func() bool {
			return inh.State() == StateAcquired
		})

		entries := logindInhibitors(t)
		found := false
		for _, e := range entries {
			if e.Who == "sway-power" && e.What == "handle-lid-switch" && e.Mode == "block" {
				found = true
			}
		}
		if !found {
			t.Fatalf("logind Inhibitors has no sway-power handle-lid-switch block entry:\n%v", entries)
		}
		if got := swayPowerLockCount(t); got != baseline+1 {
			t.Fatalf("logind sees %d sway-power locks, want %d (baseline + ours)", got, baseline+1)
		}

		// Release: closing the fd must drop the lock promptly — proves
		// release happens on fd close, not just process death.
		inh.Release()
		waitFor(t, 5*time.Second, "logind to drop the lock after fd close", func() bool {
			return swayPowerLockCount(t) == baseline
		})
	})

	t.Run("logind restart recovery", func(t *testing.T) {
		requireReal(t)
		if os.Geteuid() != 0 {
			t.Skip("restarting systemd-logind requires root")
		}

		inh := NewInhibitor(&recordingLogger{})
		defer inh.Release()
		waitFor(t, 15*time.Second, "inhibitor to reach acquired", func() bool {
			return inh.State() == StateAcquired
		})

		// Observe every state transition while holding the lock.
		var (
			mu      sync.Mutex
			sawLost bool
			done    = make(chan struct{})
		)
		go func() {
			for {
				s := inh.State()
				mu.Lock()
				if s == StateLost {
					sawLost = true
				}
				mu.Unlock()
				select {
				case <-done:
					return
				case <-time.After(10 * time.Millisecond):
				}
			}
		}()
		defer close(done)

		// Restart logind: every inhibit lock (including other
		// sway-power instances', e.g. the user's running service) dies
		// with the old logind instance.
		if out, err := exec.Command("systemctl", "--system", "restart", "systemd-logind").CombinedOutput(); err != nil {
			t.Fatalf("systemctl --system restart systemd-logind: %v: %s", err, out)
		}

		// The state machine must pass through lost (bus signal or
		// liveness poll) and re-acquire on the new logind. 5 s retry
		// interval → generous window.
		waitFor(t, 30*time.Second, "inhibitor to observe lost", func() bool {
			mu.Lock()
			defer mu.Unlock()
			return sawLost
		})
		waitFor(t, 30*time.Second, "inhibitor to re-acquire on new logind", func() bool {
			return inh.State() == StateAcquired
		})

		// No orphaned locks from the old logind instance: the new
		// logind's list shows ours plus (once the user's own daemon,
		// if any, re-acquired) the baseline.
		waitFor(t, 30*time.Second, "lock count to return to baseline+1", func() bool {
			return swayPowerLockCount(t) == baseline+1
		})
	})
}
