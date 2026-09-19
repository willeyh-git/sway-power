//go:build integration

package daemon

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/willeyh-git/sway-power/internal/preferences"
)

// TestRealPrefsUpdateWhileRunning runs a full in-process daemon — real
// Inhibitor (real system bus lock), real Monitor, real Handler, real
// PreferencesWatcher — with only the config *path* relocated to a temp
// dir via XDG_CONFIG_HOME. In-process is what makes the swap
// observable: Daemon.GetAction() is exported, no log-line archaeology.
//
// Safe tier. Brief real lock on the session, released in teardown.
func TestRealPrefsUpdateWhileRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: skipped by -short")
	}
	requireSystemBus(t)

	// Baseline before this daemon acquires its lock.
	baseline := swayPowerLockCount(t)

	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	prefsPath := filepath.Join(cfg, "sway-power", "preferences.json")
	if err := os.MkdirAll(filepath.Dir(prefsPath), 0700); err != nil {
		t.Fatalf("make config dir: %v", err)
	}
	// The initial load must come through the real file path, not the
	// defaults: the file starts at "nothing", the default is "lock".
	if err := os.WriteFile(prefsPath, []byte(`{"lid_close": "nothing"}`), 0600); err != nil {
		t.Fatalf("write preferences: %v", err)
	}

	d, err := New(false, "test")
	if err != nil {
		t.Fatalf("daemon.New: %v", err)
	}
	t.Cleanup(d.Shutdown)

	// Startup: initial load through the real file.
	if got := d.GetAction(); got != "nothing" {
		t.Fatalf("initial action from real preferences file: got %q, want %q", got, "nothing")
	}
	// And the real lock is held.
	waitFor(t, 15*time.Second, "inhibitor to reach acquired", func() bool {
		return d.GetInhibitorState() == "acquired"
	})

	// Live swap through the same atomic writer the GUI uses: real
	// mtime polling, real parse, real handler swap, monitor untouched.
	if err := preferences.Save(preferences.Preferences{LidClose: "sleep"}); err != nil {
		t.Fatalf("save sleep: %v", err)
	}
	waitFor(t, 5*time.Second, "watcher to swap action to sleep", func() bool {
		return d.GetAction() == "sleep"
	})

	// Corrupt file: must be ignored without disturbing the held lock
	// or the last-good action. There is no exported signal that the
	// poller observed the corrupt file, so elapse just over one poll
	// period (1 s) to guarantee a tick after the write — a bounded
	// wait for time, not for state.
	if err := os.WriteFile(prefsPath, []byte("{not json"), 0600); err != nil {
		t.Fatalf("write corrupt preferences: %v", err)
	}
	time.Sleep(1500 * time.Millisecond)
	if got := d.GetAction(); got != "sleep" {
		t.Fatalf("corrupt preferences file changed the action: %q, want sleep (last-good kept)", got)
	}
	if err := preferences.Save(preferences.Preferences{LidClose: "lock"}); err != nil {
		t.Fatalf("save lock: %v", err)
	}
	waitFor(t, 5*time.Second, "watcher to swap action to lock after corrupt file", func() bool {
		return d.GetAction() == "lock"
	})
	if got := d.GetInhibitorState(); got != "acquired" {
		t.Fatalf("inhibitor state after corrupt-file episode: %q, want acquired", got)
	}

	// Teardown: Shutdown releases the real lock.
	d.Shutdown()
	waitFor(t, 5*time.Second, "inhibit lock to be released on shutdown", func() bool {
		return swayPowerLockCount(t) == baseline
	})
}
