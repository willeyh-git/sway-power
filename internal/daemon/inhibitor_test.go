package daemon

import (
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// mockBus is a busConn for tests: it can simulate logind disappearing
// (lost channel) and the bus connection being dropped (connected).
type mockBus struct {
	mu           sync.Mutex
	inhibitErr   error
	inhibitCalls int
	files        []*os.File
	lostOnce     sync.Once
	lostCh       chan struct{}
	alive        bool
}

func newMockBus() *mockBus {
	return &mockBus{lostCh: make(chan struct{}), alive: true}
}

func (m *mockBus) inhibit() (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inhibitCalls++
	if !m.alive {
		// Like godbus, a call on a dropped connection errors.
		return 0, errBusDropped
	}
	if m.inhibitErr != nil {
		return 0, m.inhibitErr
	}
	// Use a real pipe so the fd is valid for syscall.Close in the
	// inhibitor (and Release). Both ends are kept open here; closing
	// the raw fd from the inhibitor is fine while we hold the files.
	//
	// Raw-fd contract (do not "simplify"): the returned fd is closed
	// with syscall.Close from the inhibitor, while m.files still holds
	// the os.File wrapping it. close() below therefore closes files
	// whose underlying fd may already be closed; that error is
	// intentional and ignored.
	r, w, err := os.Pipe()
	if err != nil {
		return 0, err
	}
	m.files = append(m.files, r, w)
	return int(w.Fd()), nil
}

func (m *mockBus) lost() <-chan struct{} {
	return m.lostCh
}

func (m *mockBus) connected() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.alive
}

func (m *mockBus) close() {
	// Unblocks lost() waiters; the real systemBusConn does the same.
	m.lostOnce.Do(func() { close(m.lostCh) })
	m.mu.Lock()
	for _, f := range m.files {
		f.Close()
	}
	m.mu.Unlock()
}

// dropLogind simulates logind losing the org.freedesktop.login1 name
// (restart/exit) while the bus connection stays alive.
func (m *mockBus) dropLogind() {
	m.lostOnce.Do(func() { close(m.lostCh) })
}

// dropBusSilent simulates a transport drop that godbus does not
// surface (it has no disconnect callback): connected() stops
// reporting the bus, but lost() stays open. Detection must come from
// the inhibitor's liveness poll.
func (m *mockBus) dropBusSilent() {
	m.mu.Lock()
	m.alive = false
	m.mu.Unlock()
}

// dropBus simulates a hard bus drop, additionally unblocking lost().
func (m *mockBus) dropBus() {
	m.dropBusSilent()
	m.lostOnce.Do(func() { close(m.lostCh) })
}

func (m *mockBus) calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.inhibitCalls
}

var (
	errNoBus      = errors.New("no system bus")
	errBusDropped = errors.New("bus connection dropped")
)

// busFactory hands out mock connections and lets tests count how many
// were opened.
type busFactory struct {
	mu    sync.Mutex
	conns []*mockBus
}

func (f *busFactory) connect() (busConn, error) {
	c := newMockBus()
	f.mu.Lock()
	f.conns = append(f.conns, c)
	f.mu.Unlock()
	return c, nil
}

func (f *busFactory) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.conns)
}

func (f *busFactory) at(i int) *mockBus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.conns[i]
}

// waitForState polls the inhibitor state until it matches or times
// out.
func waitForState(t *testing.T, i *Inhibitor, want State) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s := i.State(); s == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for state %q (state machine is persistent)", want)
}

// expectReacquire blocks until the inhibitor has reacquired the lock
// on a fresh connection (more connections than before). The
// connection count only grows after the state left acquired, so
// "count > before && state == acquired" can only be true once the
// new connection holds the lock.
func expectReacquire(t *testing.T, f *busFactory, i *Inhibitor, before int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if f.count() > before && i.State() == StateAcquired {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for reacquire: %d connections, state %q (state machine is persistent)", f.count(), i.State())
}

// newTestInhibitor builds an Inhibitor on mock connections with fast
// retry/liveness intervals.
func newTestInhibitor(t *testing.T, f *busFactory) *Inhibitor {
	t.Helper()
	i := newInhibitor(&testLogger{t}, f.connect, 20*time.Millisecond)
	t.Cleanup(i.Release)
	return i
}

func TestInhibitorAcquires(t *testing.T) {
	f := &busFactory{}
	i := newTestInhibitor(t, f)

	waitForState(t, i, StateAcquired)
	if f.count() != 1 {
		t.Errorf("expected 1 bus connection, got %d", f.count())
	}
	if f.at(0).calls() != 1 {
		t.Errorf("expected 1 Inhibit call, got %d", f.at(0).calls())
	}
}

func TestInhibitorReacquiresAfterLogindGone(t *testing.T) {
	f := &busFactory{}
	i := newTestInhibitor(t, f)

	waitForState(t, i, StateAcquired)

	// logind disappears (restart) after acquisition: the bus
	// connection is still alive, the lock is gone. Expect a re-acquire
	// on a fresh connection.
	f.at(0).dropLogind()

	expectReacquire(t, f, i, 1)

	if got := f.count(); got != 2 {
		t.Errorf("expected reacquire on a 2nd connection, got %d connections", got)
	}
	if got := i.State(); got != StateAcquired {
		t.Errorf("after logind gone: state %q, want acquired", got)
	}
	if c := f.at(1).calls(); c != 1 {
		t.Errorf("expected 1 Inhibit call on 2nd connection, got %d", c)
	}
}

// TestInhibitorSurvivesRepeatedLogindRestarts exercises the full
// persistent loop: acquired → lost → acquired must hold for every
// logind restart, not just the first.
func TestInhibitorSurvivesRepeatedLogindRestarts(t *testing.T) {
	f := &busFactory{}
	i := newTestInhibitor(t, f)

	waitForState(t, i, StateAcquired)

	for n := 0; n < 2; n++ {
		f.at(n).dropLogind()
		expectReacquire(t, f, i, n+1)
	}

	if got := f.count(); got != 3 {
		t.Errorf("expected 3 connections after 2 restarts, got %d", got)
	}
	if got := i.State(); got != StateAcquired {
		t.Errorf("after restarts: state %q, want acquired", got)
	}
}

func TestInhibitorReacquiresAfterBusDrop(t *testing.T) {
	f := &busFactory{}
	i := newTestInhibitor(t, f)

	waitForState(t, i, StateAcquired)

	// The D-Bus transport drops after acquisition, without any
	// disconnect callback (godbus has none): the loss must be caught
	// by the liveness poll, not by lost().
	f.at(0).dropBusSilent()

	expectReacquire(t, f, i, 1)
	if got := f.count(); got != 2 {
		t.Errorf("expected reacquire on a 2nd connection, got %d connections", got)
	}
	if got := i.State(); got != StateAcquired {
		t.Errorf("after bus drop: state %q, want acquired", got)
	}
}

// TestInhibitorRecoversFromDeadConnBeforeAcquire covers the
// pre-acquisition failure mode: the first connection's transport drops
// before the lock is ever acquired. The dead conn must be dropped and
// acquisition continued on a fresh connection; a dead transport never
// recovers, so without the drop the Inhibitor would retry the same
// dead conn forever.
func TestInhibitorRecoversFromDeadConnBeforeAcquire(t *testing.T) {
	var (
		mu sync.Mutex
		c1 *mockBus
	)
	connect := func() (busConn, error) {
		mu.Lock()
		defer mu.Unlock()
		if c1 == nil {
			c1 = newMockBus()
			c1.dropBusSilent() // transport drops right after connect
			return c1, nil
		}
		return newMockBus(), nil
	}

	i := newInhibitor(&testLogger{t}, connect, 20*time.Millisecond)
	t.Cleanup(i.Release)

	waitForState(t, i, StateAcquired)

	if c1.connected() {
		t.Errorf("first connection should still report dead")
	}
}

func TestInhibitorRetriesWhileUnavailable(t *testing.T) {
	// No bus at all: the Inhibitor must keep retrying (never give up
	// while the daemon lives), staying in the failed state.
	var attempts atomic.Int64
	i := newInhibitor(&testLogger{t}, func() (busConn, error) {
		attempts.Add(1)
		return nil, errNoBus
	}, 10*time.Millisecond)
	t.Cleanup(i.Release)

	time.Sleep(100 * time.Millisecond)
	if got := i.State(); got != StateFailed {
		t.Errorf("expected failed state, got %q", got)
	}
	if n := attempts.Load(); n < 3 {
		t.Errorf("expected repeated retry attempts, got %d", n)
	}
}

func TestInhibitorReleaseAfterAcquire(t *testing.T) {
	f := &busFactory{}
	i := newTestInhibitor(t, f)

	waitForState(t, i, StateAcquired)

	// Release must stop the machine; a later logind disappearance must
	// not cause any further activity.
	i.Release()
	f.at(0).dropLogind()

	before := f.count()
	time.Sleep(30 * time.Millisecond)
	if got := f.count(); got != before {
		t.Errorf("after Release: expected no new connections, got %d (was %d)", got, before)
	}
}

// TestConnectSystemBusReal connects to the real system bus and runs the
// production connectSystemBus, including the NameOwnerChanged
// subscription.
//
// Regression: the code used to pass an explicit
// WithMatchOption("type", "signal") on top of the type='signal' that
// AddMatchSignal adds internally, producing a rule with a duplicate
// type key. dbus-broker (Fedora, Arch) rejects that rule with
// "Invalid match rule"; the legacy dbus-daemon tolerates duplicates —
// which is why this never surfaced outside dbus-broker systems.
//
// Skips when no system bus is available (e.g. CI).
func TestConnectSystemBusReal(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	if _, err := dbus.SystemBusPrivate(); err != nil {
		t.Skipf("no system bus available: %v", err)
	}
	c, err := connectSystemBus()
	if err != nil {
		t.Fatalf("connectSystemBus: %v", err)
	}
	defer c.close()
}
