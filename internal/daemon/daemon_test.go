package daemon

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/willeyh-git/sway-power/internal/lid/action"
	"github.com/willeyh-git/sway-power/internal/preferences"
)

// testLogger is a minimal logger for tests.
type testLogger struct {
	t *testing.T
}

func (l *testLogger) Printf(format string, args ...interface{}) {
	l.t.Logf(format, args...)
}

// mockExec tracks calls without executing real commands.
type mockExec struct {
	mu    sync.Mutex
	calls []mockCall
	err   error
}

type mockCall struct {
	name string
	args []string
}

func (m *mockExec) setErr(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}

func (m *mockExec) getCalls() []mockCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]mockCall, len(m.calls))
	copy(cp, m.calls)
	return cp
}

// called reports whether a call with the exact name and args was
// recorded.
func (m *mockExec) called(name string, args ...string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.calls {
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
			return true
		}
	}
	return false
}

func (m *mockExec) exec(name string, args ...string) error {
	m.mu.Lock()
	m.calls = append(m.calls, mockCall{name: name, args: args})
	err := m.err
	m.mu.Unlock()
	return err
}

func (m *mockExec) reset() {
	m.mu.Lock()
	m.calls = nil
	m.err = nil
	m.mu.Unlock()
}

// actionRestore saves and restores the original action exec functions after a test.
type actionRestore struct {
	origExec     action.ExecFunc
	origSwaymsg  func(args ...string) *exec.Cmd
	origSwaylock func(args ...string) error
}

func restoreActions(r actionRestore) {
	action.Exec = r.origExec
	action.SwaymsgCmd = r.origSwaymsg
	action.Swaylock = r.origSwaylock
}

func setupMockActions(t *testing.T) (actionRestore, *mockExec) {
	t.Helper()
	m := &mockExec{}
	r := actionRestore{
		origExec:     action.Exec,
		origSwaymsg:  action.SwaymsgCmd,
		origSwaylock: action.Swaylock,
	}
	action.Exec = m.exec
	// Recording swaymsg stub: calls are tracked by mockExec; the
	// returned cmd is a no-op so .Output()/.Run() fail cleanly without
	// running any real command.
	action.SwaymsgCmd = func(args ...string) *exec.Cmd {
		m.exec("swaymsg", args...)
		return &exec.Cmd{Path: "", Process: nil}
	}
	// Recording swaylock stub: no real lock screen is launched.
	action.Swaylock = func(args ...string) error {
		return m.exec("swaylock", args...)
	}
	t.Cleanup(func() { restoreActions(r) })
	return r, m
}

func TestNewHandler(t *testing.T) {
	tests := []struct {
		name       string
		initial    action.Action
		wantAction action.Action
	}{
		{"valid lock", action.ActionLock, action.ActionLock},
		{"valid sleep", action.ActionSleep, action.ActionSleep},
		{"valid nothing", action.ActionNothing, action.ActionNothing},
		{"invalid falls back to lock", action.Action("invalid"), action.ActionLock},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(tt.initial, &testLogger{t})
			got := h.GetAction()
			if got != tt.wantAction {
				t.Errorf("GetAction() = %q, want %q", got, tt.wantAction)
			}
		})
	}
}

func TestHandlerSetAction(t *testing.T) {
	h := NewHandler(action.ActionLock, &testLogger{t})

	// Set to sleep.
	h.SetAction(action.ActionSleep)
	if got := h.GetAction(); got != action.ActionSleep {
		t.Errorf("after set sleep: %q, want %q", got, action.ActionSleep)
	}

	// Set to invalid - should keep current.
	h.SetAction(action.Action("invalid"))
	if got := h.GetAction(); got != action.ActionSleep {
		t.Errorf("after set invalid: %q, want %q", got, action.ActionSleep)
	}

	// Set to nothing.
	h.SetAction(action.ActionNothing)
	if got := h.GetAction(); got != action.ActionNothing {
		t.Errorf("after set nothing: %q, want %q", got, action.ActionNothing)
	}
}

func TestHandlerActionSwapAtomicity(t *testing.T) {
	h := NewHandler(action.ActionLock, &testLogger{t})
	done := make(chan struct{})
	errCh := make(chan error, 1)

	// Start a goroutine that continuously swaps actions.
	go func() {
		actions := []action.Action{
			action.ActionLock,
			action.ActionSleep,
			action.ActionNothing,
		}
		i := 0
		for {
			select {
			case <-done:
				return
			default:
				h.SetAction(actions[i%len(actions)])
				i++
			}
		}
	}()

	// Concurrently read actions and verify they are valid.
	const iterations = 1000
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				a := h.GetAction()
				if a.Validate() != nil {
					errCh <- &syncError{err: a, action: "GetAction"}
					return
				}
			}
		}()
	}

	wg.Wait()
	close(done)

	// Check for errors.
	select {
	case err := <-errCh:
		t.Errorf("concurrent access error: %v", err)
	default:
		// No errors.
	}
}

func TestHandlerExecuteSleep(t *testing.T) {
	_, mock := setupMockActions(t)
	mock.setErr(nil)

	h := NewHandler(action.ActionSleep, &testLogger{t})
	h.HandleLidClosed()

	a := h.GetAction()
	if a != action.ActionSleep {
		t.Errorf("expected sleep action, got %q", a)
	}

	// Verify systemctl suspend was called.
	if !mock.called("systemctl", "suspend") {
		t.Errorf("expected systemctl suspend call, got calls: %v", mock.getCalls())
	}
}

func TestHandlerExecuteLock(t *testing.T) {
	_, mock := setupMockActions(t)

	h := NewHandler(action.ActionLock, &testLogger{t})
	h.HandleLidClosed()

	// The lock action goes through the Swaylock seam: verify
	// `swaylock -f` was (mocked) launched, and that the action is
	// preserved on error.
	if !mock.called("swaylock", "-f") {
		t.Errorf("expected swaylock -f call, got calls: %v", mock.getCalls())
	}
	if got := h.GetAction(); got != action.ActionLock {
		t.Errorf("expected lock action, got %q", got)
	}
}

func TestHandlerExecuteSleepError(t *testing.T) {
	_, mock := setupMockActions(t)
	mock.setErr(&os.PathError{Op: "exec", Path: "systemctl", Err: os.ErrNotExist})

	h := NewHandler(action.ActionSleep, &testLogger{t})

	// Should not panic, should log the error.
	h.HandleLidClosed()

	a := h.GetAction()
	if a != action.ActionSleep {
		t.Errorf("expected sleep action to be preserved, got %q", a)
	}
}

func TestHandlerHandleLidOpen(t *testing.T) {
	_, mock := setupMockActions(t)

	h := NewHandler(action.ActionNothing, &testLogger{t})
	h.HandleLidOpen()

	// OnOpen for nothing calls showInternalDisplay which queries sway
	// outputs via swaymsg. Verify that query was (mocked) attempted:
	// with the no-op stub cmd, .Output() fails, so the enable path is
	// not reached — the get_outputs call is the assertion target.
	a := h.GetAction()
	if a != action.ActionNothing {
		t.Errorf("expected nothing action, got %q", a)
	}
	if !mock.called("swaymsg", "-t", "json", "get_outputs") {
		t.Errorf("expected swaymsg get_outputs call, got calls: %v", mock.getCalls())
	}
}

func TestPreferencesWatcher(t *testing.T) {
	// Create a temporary preferences file.
	tmpDir := t.TempDir()
	prefsPath := filepath.Join(tmpDir, "preferences.json")

	// Write initial preferences.
	prefs := `{"lid_close": "lock"}`
	if err := os.WriteFile(prefsPath, []byte(prefs), 0644); err != nil {
		t.Fatalf("setup: write prefs: %v", err)
	}

	// Create a handler.
	h := NewHandler(action.ActionLock, &testLogger{t})

	// Create a preferences watcher that uses our temp dir.
	pw := &PreferencesWatcher{
		prefsPath: prefsPath,
		handler:   h,
		log:       &testLogger{t},
		stopCh:    make(chan struct{}),
		done:      make(chan struct{}),
	}

	// Load initial action from our temp file.
	p, err := preferences.LoadFromPath(prefsPath)
	if err == nil && p.LidClose != "" {
		h.SetAction(action.Action(p.LidClose))
	} else {
		h.SetAction(action.ActionLock)
	}

	go pw.watch()
	defer pw.Stop()

	// Wait for watcher to start.
	time.Sleep(100 * time.Millisecond)

	// Update preferences file.
	prefs = `{"lid_close": "sleep"}`
	if err := os.WriteFile(prefsPath, []byte(prefs), 0644); err != nil {
		t.Fatalf("update prefs: %v", err)
	}

	// Wait for watcher to pick up the change.
	time.Sleep(1500 * time.Millisecond) // 1s poll + some margin

	if got := h.GetAction(); got != action.ActionSleep {
		t.Errorf("after prefs update: %q, want %q", got, action.ActionSleep)
	}
}

func TestPreferencesWatcherCorruptJSON(t *testing.T) {
	// Create a temporary preferences file.
	tmpDir := t.TempDir()
	prefsPath := filepath.Join(tmpDir, "preferences.json")

	// Write initial valid preferences.
	prefs := `{"lid_close": "lock"}`
	if err := os.WriteFile(prefsPath, []byte(prefs), 0644); err != nil {
		t.Fatalf("setup: write prefs: %v", err)
	}

	// Create a handler and a preferences watcher.
	h := NewHandler(action.ActionLock, &testLogger{t})
	pw := &PreferencesWatcher{
		prefsPath: prefsPath,
		handler:   h,
		log:       &testLogger{t},
		stopCh:    make(chan struct{}),
		done:      make(chan struct{}),
	}

	go pw.watch()
	defer pw.Stop()

	// Wait for watcher to start.
	time.Sleep(100 * time.Millisecond)

	// Corrupt the file: unparseable JSON.
	corrupt := `{"lid_close": "lock",`
	if err := os.WriteFile(prefsPath, []byte(corrupt), 0644); err != nil {
		t.Fatalf("update prefs: %v", err)
	}

	// Wait for the watcher to observe the corrupt file.
	time.Sleep(1500 * time.Millisecond)

	// On parse failure the last-good action is kept — not swapped to
	// the default.
	if got := h.GetAction(); got != action.ActionLock {
		t.Errorf("after corrupt JSON: %q, want %q", got, action.ActionLock)
	}

	// Repair the file; the watcher must pick up the new value.
	validPrefs := `{"lid_close": "sleep"}`
	if err := os.WriteFile(prefsPath, []byte(validPrefs), 0644); err != nil {
		t.Fatalf("update prefs: %v", err)
	}

	time.Sleep(1500 * time.Millisecond)

	if got := h.GetAction(); got != action.ActionSleep {
		t.Errorf("after repaired prefs: %q, want %q", got, action.ActionSleep)
	}
}

func TestPreferencesWatcherInvalidAction(t *testing.T) {
	// Create a temporary preferences file.
	tmpDir := t.TempDir()
	prefsPath := filepath.Join(tmpDir, "preferences.json")

	// Write initial valid preferences.
	prefs := `{"lid_close": "lock"}`
	if err := os.WriteFile(prefsPath, []byte(prefs), 0644); err != nil {
		t.Fatalf("setup: write prefs: %v", err)
	}

	// Create a handler.
	h := NewHandler(action.ActionLock, &testLogger{t})

	// Create a preferences watcher.
	pw := &PreferencesWatcher{
		prefsPath: prefsPath,
		handler:   h,
		log:       &testLogger{t},
		stopCh:    make(chan struct{}),
		done:      make(chan struct{}),
	}

	// Load initial action from temp file.
	p, err := preferences.LoadFromPath(prefsPath)
	if err != nil {
		t.Fatalf("load prefs: %v", err)
	}
	h.SetAction(action.Action(p.LidClose))

	go pw.watch()
	defer pw.Stop()

	// Wait for watcher to start.
	time.Sleep(100 * time.Millisecond)

	// Update preferences to invalid value.
	invalidPrefs := `{"lid_close": "invalid"}`
	if err := os.WriteFile(prefsPath, []byte(invalidPrefs), 0644); err != nil {
		t.Fatalf("update prefs: %v", err)
	}

	// Wait for watcher to pick up the change.
	time.Sleep(1500 * time.Millisecond)

	// Should still have the old action.
	if got := h.GetAction(); got != action.ActionLock {
		t.Errorf("after invalid prefs: %q, want %q", got, action.ActionLock)
	}

	// Update to valid value.
	validPrefs := `{"lid_close": "sleep"}`
	if err := os.WriteFile(prefsPath, []byte(validPrefs), 0644); err != nil {
		t.Fatalf("update prefs: %v", err)
	}

	// Wait for watcher to pick up the change.
	time.Sleep(1500 * time.Millisecond)

	// Should now have the new action.
	if got := h.GetAction(); got != action.ActionSleep {
		t.Errorf("after valid prefs: %q, want %q", got, action.ActionSleep)
	}
}

type syncError struct {
	err    interface{}
	action string
}

func (e *syncError) Error() string {
	return e.action + " returned invalid action: " + formatAction(e.err)
}

func formatAction(a interface{}) string {
	if act, ok := a.(action.Action); ok {
		return string(act)
	}
	return "unknown"
}
