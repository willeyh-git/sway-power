//go:build integration

package daemon

import (
	"syscall"
	"testing"
	"time"
)

// TestRealDaemonDeathReleasesInhibitor verifies the fd contract across a
// real process boundary: the kernel, not our code, releases the logind
// inhibit fd when the owning process dies. Subprocess daemon with an
// isolated XDG_CONFIG_HOME; the test only kills — the release path is
// the kernel's.
//
// Safe tier: no destructive side effects beyond a briefly held lock.
func TestRealDaemonDeathReleasesInhibitor(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: skipped by -short")
	}
	requireSystemBus(t)

	// Baseline before this test spawns anything: may include the
	// user's own running sway-power service.
	baseline := swayPowerLockCount(t)

	t.Run("SIGKILL releases the lock via kernel fd close", func(t *testing.T) {
		d := runDaemon(t)
		waitFor(t, 15*time.Second, "child daemon to hold the inhibit lock", func() bool {
			return swayPowerLockCount(t) > baseline
		})

		// Kill the whole process group (daemon + any children).
		if err := syscall.Kill(-d.pgid, syscall.SIGKILL); err != nil {
			t.Fatalf("SIGKILL group: %v", err)
		}
		d.waitExit(t, 5*time.Second)

		// Kernel closed the fd → logind drops the lock within a few
		// seconds, no code of ours running.
		waitFor(t, 5*time.Second, "kernel to release the inhibit fd after death", func() bool {
			return swayPowerLockCount(t) == baseline
		})
	})

	t.Run("SIGTERM exits cleanly and releases the lock", func(t *testing.T) {
		d := runDaemon(t)
		waitFor(t, 15*time.Second, "child daemon to hold the inhibit lock", func() bool {
			return swayPowerLockCount(t) > baseline
		})

		// Graceful half: the Shutdown() path — Release() closes the fd —
		// must work, not only the kernel's.
		if err := d.cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatalf("SIGTERM child: %v", err)
		}
		d.waitExit(t, 10*time.Second)
		if err := d.exitErr(); err != nil {
			t.Fatalf("child did not exit cleanly on SIGTERM: %v", err)
		}

		waitFor(t, 5*time.Second, "inhibit lock to be released on clean exit", func() bool {
			return swayPowerLockCount(t) == baseline
		})
	})
}
