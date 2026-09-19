//go:build integration

package daemon

import (
	"bufio"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/willeyh-git/sway-power/internal/bootstrap"
)

// TestRealSystemdUserService is the systemd ↔ daemon boundary as a black
// box: the same four commands `sway-power install` runs (write unit,
// daemon-reload, import-environment, enable --now), observed from the
// outside — including proof that the *unit's child* holds the logind
// lock, not just the test.
//
// Opt-in (SWAY_POWER_REAL_TESTS=1): it installs, and on a machine where
// the user's own sway-power service is installed, it replaces that unit
// with one pointing at the test binary for the duration of the test and
// restores the original unit in teardown.
func TestRealSystemdUserService(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: skipped by -short")
	}
	requireReal(t)
	bin := builtSwayPower(t)

	// Skip when there is no systemd user manager to drive.
	if out, err := exec.Command("systemctl", "--user", "is-system-running").CombinedOutput(); err != nil {
		t.Skipf("no systemd user session: %v: %s", err, out)
	} else if state := strings.TrimSpace(string(out)); state != "running" && state != "degraded" {
		t.Skipf("systemd user manager state %q, want running/degraded", state)
	}

	// --- Snapshot the user's real installation before touching anything.
	preTest := swayPowerLockCount(t) // incl. the real service's lock, if any
	wasInstalled := bootstrap.IsInstalled()
	var originalExec string
	if wasInstalled {
		originalExec = originalExecStart(t)
	}
	wasActive := isActiveUnit(t)

	// Remove the user's unit now (if present) so the test installs on a
	// clean slate. Teardown restores exactly what was found.
	if wasInstalled {
		if err := bootstrap.Uninstall(); err != nil {
			t.Fatalf("uninstall user unit before test: %v", err)
		}
	}
	t.Cleanup(func() {
		if wasInstalled {
			// Bootstrap with the original ExecStart path rewrites the
			// original unit content, re-enables it (Uninstall's
			// `disable --now` removed the wants symlink) and restarts
			// it.
			if err := bootstrap.Bootstrap(originalExec); err != nil {
				t.Errorf("restore original unit: %v", err)
			}
			// A service the user had installed but deliberately
			// stopped must stay stopped.
			if !wasActive {
				if out, err := exec.Command("systemctl", "--user", "stop", bootstrap.UnitName()).CombinedOutput(); err != nil {
					t.Errorf("stop restored unit (was inactive pre-test): %v: %s", err, out)
				}
			}
		} else {
			if err := bootstrap.Uninstall(); err != nil {
				t.Errorf("uninstall unit: %v", err)
			}
		}
		// Never leave a unit or a lock behind: the machine must end in
		// its pre-test state.
		if got := bootstrap.IsInstalled(); got != wasInstalled {
			t.Errorf("unit presence after teardown: %v, want %v (pre-test state)", got, wasInstalled)
		}
		// WaitE: cleanup may not call Fatal/FailNow.
		waitForE(t, 15*time.Second, "inhibit locks to return to pre-test count", func() bool {
			return swayPowerLockCount(t) == preTest
		})
	})

	// Baseline after the slate is clean and the stopped service's lock
	// has been dropped: any sway-power lock present now is from a stray
	// non-service instance; the unit must add exactly one.
	baseline := stableLockCount(t)

	// --- Install the test binary as the user service.
	if err := bootstrap.Bootstrap(bin); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	// Unit file: ExecStart points at the built binary.
	unitPath, err := bootstrap.UnitPath()
	if err != nil {
		t.Fatalf("unit path: %v", err)
	}
	content, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("read installed unit: %v", err)
	}
	if !strings.Contains(string(content), "ExecStart="+bin+" daemon") {
		t.Errorf("unit file lacks ExecStart=%s daemon:\n%s", bin, content)
	}

	// The unit is active ...
	waitFor(t, 15*time.Second, "sway-power.service to become active", func() bool {
		return isActiveUnit(t)
	})

	// ... and its child holds the logind lock — not just the test.
	waitFor(t, 15*time.Second, "service's daemon child to hold the inhibit lock", func() bool {
		return swayPowerLockCount(t) == baseline+1
	})

	// import-environment: the session variables the user manager
	// received from this session. Only assertable when the test driver
	// itself runs in a Sway session.
	if wd := os.Getenv("WAYLAND_DISPLAY"); wd != "" {
		out, err := exec.Command("systemctl", "--user", "show-environment").Output()
		if err != nil {
			t.Fatalf("show-environment: %v", err)
		}
		if !strings.Contains(string(out), "WAYLAND_DISPLAY="+wd) {
			t.Errorf("show-environment lacks WAYLAND_DISPLAY=%s:\n%s", wd, out)
		}
	}
}

// originalExecStart reads the installed unit's ExecStart path — the
// absolute path of the user's real binary, which teardown passes back to
// bootstrap.Bootstrap to restore the original unit verbatim.
func originalExecStart(t *testing.T) string {
	t.Helper()
	path, err := bootstrap.UnitPath()
	if err != nil {
		t.Fatalf("unit path: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open unit: %v", err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "ExecStart=") {
			// Template: "ExecStart=<abs path> daemon".
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return strings.TrimPrefix(fields[1], "ExecStart=")
			}
		}
	}
	t.Fatalf("installed unit has no ExecStart line")
	return ""
}

// isActiveUnit reports whether sway-power.service is active in the user
// manager.
func isActiveUnit(t *testing.T) bool {
	t.Helper()
	out, err := exec.Command("systemctl", "--user", "is-active", bootstrap.UnitName()).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "active"
}

// stableLockCount returns the lock count once it has been unchanged for
// a second — i.e. after a stopped service's lock has drained.
func stableLockCount(t *testing.T) int {
	t.Helper()
	prev := swayPowerLockCount(t)
	since := time.Now()
	deadline := time.Now().Add(30 * time.Second)
	for {
		time.Sleep(200 * time.Millisecond)
		cur := swayPowerLockCount(t)
		if cur != prev {
			prev, since = cur, time.Now()
		} else if time.Since(since) >= time.Second {
			return prev
		}
		if time.Now().After(deadline) {
			t.Fatalf("inhibit lock count never stabilized")
		}
	}
}

// waitForE is waitFor with Errorf instead of Fatalf: cleanup functions
// may not call FailNow.
func waitForE(t *testing.T, timeout time.Duration, what string, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if fn() {
			return
		}
		if time.Now().After(deadline) {
			t.Errorf("timed out waiting for %s", what)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}
