package daemon

import (
	"fmt"
	"sync"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
)

// Inhibitor manages a logind inhibit lock via D-Bus.
//
// It is a persistent state machine:
//
//	ACQUIRING → ACQUIRED
//	              ↓ (bus connection dropped / logind disappeared)
//	             LOST → ACQUIRING (retry until reacquired)
//
// A missing or unavailable system bus is a failure state, not a
// constructor error: the Inhibitor retries every 5 seconds until it
// acquires the lock or Release() is called. Once acquired, the
// inhibitor keeps watching for the D-Bus connection or logind
// disappearing and re-acquires automatically; it never permanently
// gives up while the daemon lives.
type Inhibitor struct {
	log   Logger
	mu    sync.RWMutex
	state State
	fd    int
	conn  busConn
	// liveness ticks are only consumed while holding the lock
	// (watchWhileAcquired); unconsumed ticks while acquiring/failed are
	// harmless (the ticker buffers a single tick).
	retry    *time.Ticker
	liveness *time.Ticker
	cancel   chan struct{}
	done     chan struct{}

	// newConn builds a bus connection to logind. Read-only after
	// construction; the real implementation dials the system bus.
	newConn func() (busConn, error)

	retryEvery time.Duration
}

// retryInterval is how long to wait between failed acquisition
// attempts. While failed, logind still handles the lid — that
// double-handler window is the documented exception, so retrying is
// the point.
const retryInterval = 5 * time.Second

// livenessInterval is how often a held acquisition is checked against
// the live connection. The lost() channel catches logind
// disappearance immediately; this poll catches a silently-dropped
// D-Bus connection (godbus has no disconnect callback).
const livenessInterval = 250 * time.Millisecond

// State represents the inhibitor's current state.
type State string

const (
	StateAcquiring State = "acquiring"
	StateAcquired  State = "acquired"
	StateLost      State = "lost"
	StateFailed    State = "failed"
)

// Logger is the minimal logging interface for the daemon.
type Logger interface {
	Printf(format string, args ...interface{})
}

// busConn abstracts a live D-Bus connection to logind so that tests
// can simulate the bus or logind disappearing after acquisition.
type busConn interface {
	// inhibit requests the "handle-lid-switch" block lock and returns
	// the received fd. Closing that fd releases the lock.
	inhibit() (fd int, err error)
	// lost is closed when logind loses the org.freedesktop.login1
	// bus name (logind restart/disappearance). It is also closed
	// when the connection is closed.
	lost() <-chan struct{}
	// connected reports whether the bus transport is still alive.
	connected() bool
	// close drops the connection and unblocks lost().
	close()
}

// NewInhibitor creates a new Inhibitor and starts acquiring the logind
// "handle-lid-switch" block lock in the background. The system bus or
// logind may not be ready yet at daemon startup, so the acquire is
// retried on a 5s tick until it succeeds. Once acquired, the
// Inhibitor watches for the D-Bus connection or logind disappearing
// and re-acquires; a held lock is never silently abandoned.
// NewInhibitor never fails: bus unavailability is a transient state
// inside the Inhibitor, not a startup error. Call Release() when done.
func NewInhibitor(log Logger) *Inhibitor {
	return newInhibitor(log, connectSystemBus, retryInterval)
}

func newInhibitor(log Logger, newConn func() (busConn, error), retryEvery time.Duration) *Inhibitor {
	i := &Inhibitor{
		log:        log,
		fd:         -1, // 0 is a live fd; a fresh inhibitor holds none
		state:      StateAcquiring,
		retry:      time.NewTicker(retryEvery),
		liveness:   time.NewTicker(livenessInterval),
		cancel:     make(chan struct{}),
		done:       make(chan struct{}),
		newConn:    newConn,
		retryEvery: retryEvery,
	}

	// Start the state machine.
	go i.run()

	return i
}

// run is the persistent state machine:
//
//	ACQUIRING → (ok)   ACQUIRED → (loss) LOST → ACQUIRING ...
//	ACQUIRING → (err) FAILED   → ACQUIRING (every retryEvery)
func (i *Inhibitor) run() {
	defer close(i.done)
	defer i.retry.Stop()
	defer i.liveness.Stop()

	for {
		if i.canceled() {
			return
		}

		i.setState(StateAcquiring)
		if err := i.acquireOnce(); err != nil {
			i.setState(StateFailed)
			// Log prominently: while we are in failed, the user gets
			// two handlers for the lid (us and logind).
			i.log.Printf("inhibitor: ERROR: could not acquire lid inhibit lock: %v (retrying in %s)", err, i.retryEvery)
			select {
			case <-i.cancel:
				return
			case <-i.retry.C:
			}
			continue
		}

		i.setState(StateAcquired)
		i.mu.RLock()
		fd := i.fd
		i.mu.RUnlock()
		i.log.Printf("inhibitor: acquired lid inhibit lock (fd=%d)", fd)

		// Hold the lock until the connection/logind is lost or the
		// inhibitor is released.
		if i.watchWhileAcquired() {
			// Canceled: Release() owns fd/conn shutdown after done.
			return
		}

		// The lock is represented by an FD owned by logind and
		// associated with that FD by systemd; once the bus
		// connection or logind itself is gone, the FD is inert and
		// logind handles the lid again. Drop the dead resources and
		// immediately re-enter acquisition — we never give up while
		// the daemon lives.
		i.setState(StateLost)
		i.log.Printf("inhibitor: D-Bus connection or logind lost while holding the inhibit lock; releasing lock and re-acquiring")
		i.releaseFd()
		i.dropConn()
	}
}

func (i *Inhibitor) setState(s State) {
	i.mu.Lock()
	i.state = s
	i.mu.Unlock()
}

func (i *Inhibitor) canceled() bool {
	select {
	case <-i.cancel:
		return true
	default:
		return false
	}
}

// watchWhileAcquired blocks while the lock is held until either the
// bus connection drops, logind disappears (lost() channel), or the
// inhibitor is canceled. Returns true only if canceled.
func (i *Inhibitor) watchWhileAcquired() bool {
	conn := i.conn
	for {
		select {
		case <-i.cancel:
			return true
		case <-conn.lost():
			return false
		case <-i.liveness.C:
			// godbus exposes no disconnect callback; a dropped
			// connection is detected here.
			if !conn.connected() {
				return false
			}
		}
	}
}

// acquireOnce opens a connection if needed, then makes a single
// Inhibit() call on logind via D-Bus.
//
// logind is a system service: org.freedesktop.login1 lives on the
// system bus, not the session bus. An unprivileged Inhibit() goes
// through polkit (org.freedesktop.login1.inhibit-block-handle-lid-
// switch).
//
// Called only from the run loop.
func (i *Inhibitor) acquireOnce() error {
	if i.conn != nil && !i.conn.connected() {
		// The transport died before the lock was acquired (bus daemon
		// restart, ...). Dropping is mandatory: a dead connection never
		// recovers and would make every retry fail on the same dead
		// conn. A live conn is NOT dropped here: if logind itself
		// restarted, the same conn is still valid and the next call
		// lands on the new logind.
		i.dropConn()
	}

	if i.conn == nil {
		conn, err := i.newConn()
		if err != nil {
			return err
		}
		i.conn = conn
	}

	fd, err := i.conn.inhibit()
	if err != nil {
		return err
	}

	i.mu.Lock()
	// Close any previous fd before installing the new one.
	if i.fd >= 0 {
		syscall.Close(i.fd)
	}
	i.fd = fd
	i.mu.Unlock()

	return nil
}

// releaseFd closes the held inhibit fd, releasing the lock.
func (i *Inhibitor) releaseFd() {
	i.mu.Lock()
	if i.fd >= 0 {
		syscall.Close(i.fd)
		i.fd = -1
	}
	i.mu.Unlock()
}

// dropConn closes the bus connection after a loss; the next acquire
// opens a fresh one.
func (i *Inhibitor) dropConn() {
	if i.conn != nil {
		i.conn.close()
		i.conn = nil
	}
}

// connectSystemBus connects to the system bus (auth + Hello).
//
// The system bus and logind can be briefly unavailable during session
// startup; the retry loop handles that by calling this again on the
// next tick.
func connectSystemBus() (busConn, error) {
	// SystemBusPrivate returns a connection that is not ready to use:
	// Dial never authenticates, so Auth and Hello are mandatory before
	// the first call.
	conn, err := dbus.SystemBusPrivate()
	if err != nil {
		return nil, fmt.Errorf("connect to system bus: %w", err)
	}
	if err := conn.Auth(nil); err != nil {
		conn.Close()
		return nil, fmt.Errorf("system bus auth: %w", err)
	}
	if err := conn.Hello(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("system bus Hello: %w", err)
	}

	c := &systemBusConn{
		c:      conn,
		lostCh: make(chan struct{}),
	}

	// Watch for logind losing its bus name: if logind restarts or
	// exits, the inhibit lock (which systemd tracks via the fd
	// association) is gone even though our bus connection is still
	// alive.
	sigCh := make(chan *dbus.Signal)
	conn.Signal(sigCh)
	err = conn.AddMatchSignal(
		dbus.WithMatchOption("type", "signal"),
		dbus.WithMatchInterface("org.freedesktop.DBus"),
		dbus.WithMatchMember("NameOwnerChanged"),
	)
	if err != nil {
		conn.RemoveSignal(sigCh)
		conn.Close()
		return nil, fmt.Errorf("subscribe to NameOwnerChanged: %w", err)
	}
	go c.watchLogind(sigCh)

	return c, nil
}

// systemBusConn is a busConn backed by the real system bus.
type systemBusConn struct {
	c        *dbus.Conn
	lostOnce sync.Once
	lostCh   chan struct{}
}

// watchLogind fires the lost channel when logind loses the
// org.freedesktop.login1 name. The signal channel closes when the
// bus connection is closed, ending this goroutine.
func (c *systemBusConn) watchLogind(sigCh <-chan *dbus.Signal) {
	for sig := range sigCh {
		if sig == nil {
			continue
		}
		if sig.Name != "org.freedesktop.DBus.NameOwnerChanged" {
			continue
		}
		// Body: [name string, old_owner string, new_owner string].
		// An empty new_owner means the name was released (logind
		// died/restarted).
		if len(sig.Body) < 3 {
			continue
		}
		name, _ := sig.Body[0].(string)
		newOwner, _ := sig.Body[2].(string)
		if name == "org.freedesktop.login1" && newOwner == "" {
			c.lostOnce.Do(func() { close(c.lostCh) })
		}
	}
}

func (c *systemBusConn) inhibit() (int, error) {
	call := c.c.Object("org.freedesktop.login1", "/org/freedesktop/login1").
		Call("org.freedesktop.login1.Manager.Inhibit", 0,
			"handle-lid-switch", "sway-power",
			"Handle lid close ourselves", "block")
	if call.Err != nil {
		return 0, fmt.Errorf("Inhibit: %w", call.Err)
	}

	if len(call.Body) < 1 {
		return 0, fmt.Errorf("Inhibit reply has no body")
	}
	// Received fds arrive as dbus.UnixFD: the int32 fd number local to
	// this process after the SCM_RIGHTS transfer. Closing that fd
	// releases the lock.
	fd, ok := call.Body[0].(dbus.UnixFD)
	if !ok {
		return 0, fmt.Errorf("Inhibit reply body is not a fd: %T", call.Body[0])
	}
	return int(fd), nil
}

func (c *systemBusConn) lost() <-chan struct{} {
	return c.lostCh
}

func (c *systemBusConn) connected() bool {
	return c.c.Connected()
}

func (c *systemBusConn) close() {
	// Unblock anyone waiting on lost(), then drop the connection.
	c.lostOnce.Do(func() { close(c.lostCh) })
	c.c.Close()
}

// State returns the current inhibitor state.
func (i *Inhibitor) State() State {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.state
}

// Release stops the state machine, closes the inhibit fd (releasing
// the lock) and the D-Bus connection. Safe to call multiple times.
func (i *Inhibitor) Release() {
	select {
	case <-i.cancel:
		return // already closed
	default:
	}
	close(i.cancel)

	// Wait for the state machine to finish; after this, only this
	// goroutine touches fd/conn.
	<-i.done

	i.mu.Lock()
	if i.fd >= 0 {
		syscall.Close(i.fd)
		i.fd = -1
	}
	i.mu.Unlock()

	if i.conn != nil {
		i.conn.close()
	}
}
