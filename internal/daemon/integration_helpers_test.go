//go:build integration

// Real integration tests: shared helpers.
//
// These tests run against the machine/session they are invoked from:
// real system bus, real logind, real evdev, real systemd user manager,
// real sway. Every test auto-skips (never fails) when its prerequisite
// is missing, and none of them mutates the user's real
// ~/.config/sway-power — every daemon that runs gets an isolated
// XDG_CONFIG_HOME.
//
// Tiers:
//
//   - safe:   run with  go test -tags=integration -run '^TestReal' ./internal/daemon
//     Side effect: a brief, released logind "handle-lid-switch"
//     block lock per scenario.
//   - opt-in: additionally require SWAY_POWER_REAL_TESTS=1 (logind
//     restart, user-service install, interactive lid toggle).
package daemon

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// realEnvVar gates the destructive/interactive scenarios on top of the
// //go:build integration tag (defense in depth against CI/cron).
const realEnvVar = "SWAY_POWER_REAL_TESTS"

// requireReal skips unless SWAY_POWER_REAL_TESTS=1 is set.
func requireReal(t *testing.T) {
	t.Helper()
	if os.Getenv(realEnvVar) != "1" {
		t.Skipf("set %s=1 to run destructive/interactive scenarios", realEnvVar)
	}
}

// testBinary is the absolute path of the sway-power binary built once
// by TestMain for the subprocess scenarios. Empty when the build
// failed, in which case builtSwayPower skips.
var testBinary string

// TestMain builds ./cmd/sway-power once for all subprocess scenarios.
// A failed build only disables the scenarios that need the binary.
func TestMain(m *testing.M) {
	bin, dir, err := buildTestBinary()
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: could not build sway-power: %v (subprocess scenarios will skip)\n", err)
	} else {
		testBinary = bin
	}
	code := m.Run()
	if dir != "" {
		os.RemoveAll(dir)
	}
	os.Exit(code)
}

func buildTestBinary() (bin, dir string, err error) {
	dir, err = os.MkdirTemp("", "sway-power-int-")
	if err != nil {
		return "", "", fmt.Errorf("make temp dir: %w", err)
	}
	bin = filepath.Join(dir, "sway-power")
	var out []byte
	if out, err = exec.Command("go", "build", "-o", bin, "github.com/willeyh-git/sway-power/cmd/sway-power").CombinedOutput(); err != nil {
		os.RemoveAll(dir)
		return "", "", fmt.Errorf("go build: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return bin, dir, nil
}

// builtSwayPower returns the path of the built test binary.
func builtSwayPower(t *testing.T) string {
	t.Helper()
	if testBinary == "" {
		t.Skip("sway-power test binary was not built (see TestMain output)")
	}
	return testBinary
}

// requireSystemBus skips when the system bus (and thus logind) is not
// reachable from this process.
func requireSystemBus(t *testing.T) {
	t.Helper()
	if os.Getenv("XDG_RUNTIME_DIR") == "" {
		t.Skip("XDG_RUNTIME_DIR not set: no D-Bus, no system bus")
	}
	conn, err := dbus.SystemBus()
	if err != nil {
		t.Skipf("system bus unavailable: %v", err)
	}
	conn.Close()
}

// waitFor polls fn every 100 ms until it returns true or the timeout
// expires. The polling idiom used throughout the fake-backed
// integration tests; no bare sleeps as synchronization.
func waitFor(t *testing.T, timeout time.Duration, what string, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if fn() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// logindInhibitorEntry is one row of logind's inhibit list.
type logindInhibitorEntry struct {
	Who  string
	What string
	Why  string
	Mode string
}

// logindInhibitors calls org.freedesktop.login1.Manager.Inhibitors on
// the real system bus. The assertion primitive for every inhibitor
// scenario — no parsing of `systemd-inhibit` output.
func logindInhibitors(t *testing.T) []logindInhibitorEntry {
	t.Helper()
	conn, err := dbus.SystemBus()
	if err != nil {
		t.Fatalf("system bus: %v", err)
	}
	defer conn.Close()

	var raw [][4]string
	call := conn.Object("org.freedesktop.login1", "/org/freedesktop/login1").
		Call("org.freedesktop.login1.Manager.Inhibitors", 0)
	if err := call.Store(&raw); err != nil {
		t.Fatalf("Inhibitors: %v", err)
	}
	out := make([]logindInhibitorEntry, len(raw))
	for i, e := range raw {
		out[i] = logindInhibitorEntry{Who: e[0], What: e[1], Why: e[2], Mode: e[3]}
	}
	return out
}

// swayPowerLockCount counts logind inhibit entries taken by sway-power.
//
// Count-based (not identity-based): logind's Inhibit reply carries no
// caller identifier, so two sway-power instances (e.g. the user's
// running service and this test) are indistinguishable rows. Tests
// record a baseline, then assert deltas and exact returns to it.
func swayPowerLockCount(t *testing.T) int {
	t.Helper()
	n := 0
	for _, e := range logindInhibitors(t) {
		if e.Who == "sway-power" && e.What == "handle-lid-switch" {
			n++
		}
	}
	return n
}

// recordingLogger is a Logger that keeps its lines in a buffer for
// post-hoc inspection.
type recordingLogger struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *recordingLogger) Printf(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buf.WriteString(fmt.Sprintf(format, args...))
	l.buf.WriteByte('\n')
}

func (l *recordingLogger) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

// runningDaemon is a sway-power daemon child process with an isolated
// XDG_CONFIG_HOME (a valid preferences.json with action "nothing"),
// its own process group, and captured stderr.
type runningDaemon struct {
	cmd      *exec.Cmd
	pgid     int
	stderr   *bytes.Buffer
	mu       sync.Mutex
	waitErr  error
	exitDone chan struct{}
}

// runDaemon spawns `sway-power daemon` with an isolated config dir and
// returns once it has started. Teardown SIGTERMs the process group and
// SIGKILLs stragglers, so no child survives a test.
func runDaemon(t *testing.T) *runningDaemon {
	t.Helper()
	bin := builtSwayPower(t)

	cfg := t.TempDir()
	prefsDir := filepath.Join(cfg, "sway-power")
	if err := os.MkdirAll(prefsDir, 0700); err != nil {
		t.Fatalf("make config dir: %v", err)
	}
	// A valid file so the child's initial load finds nothing to log.
	if err := os.WriteFile(filepath.Join(prefsDir, "preferences.json"), []byte(`{"lid_close": "nothing"}`), 0600); err != nil {
		t.Fatalf("write preferences: %v", err)
	}

	cmd := exec.Command(bin, "daemon")
	// Keep the child's cwd out of the test's temp dirs too.
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+cfg)
	// Own process group: the death scenario kills the whole group.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	d := &runningDaemon{
		cmd:      cmd,
		pgid:     cmd.Process.Pid,
		stderr:   stderr,
		exitDone: make(chan struct{}),
	}
	go func() {
		d.waitErr = cmd.Wait()
		close(d.exitDone)
	}()
	t.Cleanup(func() { d.stop(t) })
	return d
}

// stop SIGTERMs the child's process group, waits up to 5 s for exit,
// then SIGKILLs the group. Safe to call from t.Cleanup.
func (d *runningDaemon) stop(t *testing.T) {
	t.Helper()
	_ = syscall.Kill(-d.pgid, syscall.SIGTERM)
	d.waitExitE(t, 5*time.Second, "daemon child to exit on SIGTERM")
	// Stragglers: SIGKILL the whole group.
	_ = syscall.Kill(-d.pgid, syscall.SIGKILL)
	d.waitExitE(t, 5*time.Second, "daemon child to die on SIGKILL")
}

// waitExit blocks until the child has exited or the timeout expires.
func (d *runningDaemon) waitExit(t *testing.T, timeout time.Duration) {
	t.Helper()
	select {
	case <-d.exitDone:
	case <-time.After(timeout):
		t.Fatalf("daemon child did not exit within %s", timeout)
	}
}

// waitExitE is waitExit with Errorf instead of Fatalf: t.Cleanup may
// not call FailNow.
func (d *runningDaemon) waitExitE(t *testing.T, timeout time.Duration, what string) {
	t.Helper()
	select {
	case <-d.exitDone:
	case <-time.After(timeout):
		t.Errorf("timed out waiting for %s", what)
	}
}

// exitErr returns the child's Wait error; valid only after waitExit.
func (d *runningDaemon) exitErr() error {
	return d.waitErr
}

// logCount returns how many times s occurs in the captured stderr.
// With the monitor's dedup every occurrence of a handler line is a
// real transition, so "occurred once more than when observed" is the
// race-free "happened after" check.
func (d *runningDaemon) logCount(s string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return bytes.Count(d.stderr.Bytes(), []byte(s))
}

// alive reports whether the child process is still running.
func (d *runningDaemon) alive() bool {
	select {
	case <-d.exitDone:
		return false
	default:
	}
	return d.cmd.Process.Signal(syscall.Signal(0)) == nil
}
