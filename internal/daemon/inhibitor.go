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
// It is a recoverable state machine: acquiring → acquired | failed.
// A missing or unavailable session bus is a failure state, not a
// constructor error: the Inhibitor retries every 5 seconds until it
// acquires the lock or Release() is called. It never permanently gives
// up while the daemon lives.
type Inhibitor struct {
	conn   *dbus.Conn
	fd     int
	log    Logger
	mu     sync.RWMutex
	state  State
	retry  *time.Ticker
	cancel chan struct{}
	done   chan struct{}
}

// State represents the inhibitor's current state.
type State string

const (
	StateAcquiring State = "acquiring"
	StateAcquired  State = "acquired"
	StateFailed    State = "failed"
)

// Logger is the minimal logging interface for the daemon.
type Logger interface {
	Printf(format string, args ...interface{})
}

// NewInhibitor creates a new Inhibitor and starts acquiring the logind
// "handle-lid-switch" block lock in the background. The session bus or
// logind may not be ready yet at daemon startup, so the acquire is
// retried on a 5s tick until it succeeds. NewInhibitor never fails:
// bus unavailability is a transient state inside the Inhibitor, not a
// startup error. Call Release() when done.
func NewInhibitor(log Logger) *Inhibitor {
	i := &Inhibitor{
		fd:     -1, // 0 is a live fd; a fresh inhibitor holds none
		log:    log,
		state:  StateAcquiring,
		retry:  time.NewTicker(5 * time.Second),
		cancel: make(chan struct{}),
		done:   make(chan struct{}),
	}

	// Start retry loop.
	go i.retryLoop()

	return i
}

// retryLoop attempts the acquire immediately, then on every 5s tick,
// until it succeeds. While failed, logind still handles the lid — that
// double-handler window is the documented exception, so retrying is the
// point. On success the lock is held until Release() and the retry
// ticker stops.
func (i *Inhibitor) retryLoop() {
	defer close(i.done)
	defer i.retry.Stop()

	for {
		i.setState(StateAcquiring)
		if err := i.acquireOnce(); err != nil {
			i.setState(StateFailed)
			// Log prominently: while we are in failed, the user gets
			// two handlers for the lid (us and logind).
			i.log.Printf("inhibitor: ERROR: could not acquire lid inhibit lock: %v (retrying in 5s)", err)
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
		// Lock held; exit the loop. The fd is released by Release()
		// (or by the kernel on crash/kill).
		return
	}
}

func (i *Inhibitor) setState(s State) {
	i.mu.Lock()
	i.state = s
	i.mu.Unlock()
}

// acquireOnce connects to the session bus if needed, then makes a single
// Inhibit() call on logind via D-Bus.
func (i *Inhibitor) acquireOnce() error {
	if err := i.ensureConn(); err != nil {
		return err
	}

	call := i.conn.Object("org.freedesktop.login1", "/org/freedesktop/login1").
		Call("org.freedesktop.login1.Manager.Inhibit", 0,
			"handle-lid-switch", "sway-power",
			"Handle lid close ourselves", "block")
	if call.Err != nil {
		return fmt.Errorf("Inhibit: %w", call.Err)
	}

	if len(call.Body) < 1 {
		return fmt.Errorf("Inhibit reply has no body")
	}
	// Received fds arrive as dbus.UnixFD: the int32 fd number local to
	// this process after the SCM_RIGHTS transfer. Closing that fd
	// releases the lock.
	fd, ok := call.Body[0].(dbus.UnixFD)
	if !ok {
		return fmt.Errorf("Inhibit reply body is not a fd: %T", call.Body[0])
	}

	i.mu.Lock()
	// Close any previous fd before installing the new one.
	if i.fd >= 0 {
		syscall.Close(i.fd)
	}
	i.fd = int(fd)
	i.mu.Unlock()

	return nil
}

// ensureConn connects to the session bus (auth + Hello) when we don't
// have a live connection, reconnecting after a dropped one.
//
// The session bus and logind can be briefly unavailable during session
// startup; the retry loop handles that by calling this again on the
// next tick.
//
// Called only from the retry loop; Release() only touches conn after
// the loop has exited (done is closed), so no lock is needed.
func (i *Inhibitor) ensureConn() error {
	if i.conn != nil && i.conn.Connected() {
		return nil
	}
	if i.conn != nil {
		i.conn.Close()
	}

	// SessionBusPrivate returns a connection that is not ready to use:
	// Dial never authenticates, so Auth and Hello are mandatory before
	// the first call.
	conn, err := dbus.SessionBusPrivate()
	if err != nil {
		return fmt.Errorf("connect to session bus: %w", err)
	}
	if err := conn.Auth(nil); err != nil {
		conn.Close()
		return fmt.Errorf("session bus auth: %w", err)
	}
	if err := conn.Hello(); err != nil {
		conn.Close()
		return fmt.Errorf("session bus Hello: %w", err)
	}

	i.conn = conn
	return nil
}

// State returns the current inhibitor state.
func (i *Inhibitor) State() State {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.state
}

// Release stops the retry loop, closes the inhibit fd (releasing the
// lock) and the D-Bus connection. Safe to call exactly once.
func (i *Inhibitor) Release() {
	select {
	case <-i.cancel:
		return // already closed
	default:
	}
	close(i.cancel)

	// Wait for the retry loop to finish; after this, only this
	// goroutine touches fd/conn.
	<-i.done

	i.mu.Lock()
	if i.fd >= 0 {
		syscall.Close(i.fd)
		i.fd = -1
	}
	i.mu.Unlock()

	if i.conn != nil {
		i.conn.Close()
	}
}
