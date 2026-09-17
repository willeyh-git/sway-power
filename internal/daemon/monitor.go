package daemon

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// LidState represents the current lid state.
type LidState int

const (
	LidOpen   LidState = iota // Lid is open
	LidClosed                 // Lid is closed
)

func (s LidState) String() string {
	switch s {
	case LidOpen:
		return "open"
	case LidClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// evdev constants for the lid switch: a SW_LID switch event carries
// value 1 when the lid is open and 0 when it is closed.
const (
	evSwitch = 7 // EV_SW
	swLid    = 0x0123
)

const inputEventSize = 16 // sizeof(struct input_event)

// Monitor watches the lid switch state.
//
// It has two sources of truth:
//
//   - the evdev input device (/dev/input/eventN discovered via
//     /sys/class/input, name "Lid Switch"), consumed as a real input event
//     stream — zero-latency transitions;
//   - a state file (/proc/acpi/button/lid/*/state or the sysfs
//     lid_switch attribute), read once for the initial state and then
//     re-read every 500 ms as a backstop so no transition can be missed
//     even if the event stream stalls.
//
// Both sources feed the same change detector, so the callback fires at
// most once per actual state change.
type Monitor struct {
	eventDevice string // /dev/input/eventN backed by the lid switch; "" if none
	stateFile   string // state file for initial state + polling backstop; "" if none
	log         Logger
	debug       bool
	callback    func(LidState) // called on state change or initial state
	stopCh      chan struct{}
	done        chan struct{}
}

// NewMonitor creates a new lid state monitor.
//
// The callback is called with the initial state (from the state file) as
// soon as it is read, then on each subsequent state change. If only an
// evdev device is found (no state file), the first callback happens on
// the first lid transition instead.
//
// The monitor polls the state file at 500 ms intervals as a backstop;
// debug enables the chatty per-read log.
func NewMonitor(log Logger, debug bool, callback func(LidState)) *Monitor {
	return newMonitor(log, debug, callback, "/", "/dev")
}

// newMonitor is NewMonitor with injectable roots, for tests: sysRoot
// replaces "/" for the /sys and /proc trees, devRoot replaces /dev.
func newMonitor(log Logger, debug bool, callback func(LidState), sysRoot, devRoot string) *Monitor {
	eventDevice, stateFile := findLidSources(log, sysRoot, devRoot)
	if eventDevice == "" && stateFile == "" {
		log.Printf("lid: could not find lid switch (no evdev device, no state file)")
		return nil
	}

	m := &Monitor{
		eventDevice: eventDevice,
		stateFile:   stateFile,
		log:         log,
		debug:       debug,
		callback:    callback,
		stopCh:      make(chan struct{}),
		done:        make(chan struct{}),
	}

	// Start monitoring (initial state is read internally).
	go m.run()

	return m
}

// run drives the monitor: it fans out the two reader loops and applies
// their states to the callback, deduplicating repeats.
func (m *Monitor) run() {
	defer close(m.done)

	states := make(chan LidState, 8)
	var wg sync.WaitGroup

	var eventFile *os.File
	if m.eventDevice != "" {
		f, err := os.Open(m.eventDevice)
		if err != nil {
			m.log.Printf("lid: cannot open %s: %v (state-file polling only)", m.eventDevice, err)
		} else {
			eventFile = f
			wg.Add(1)
			go func() {
				defer wg.Done()
				m.readEvdev(f, states)
			}()
		}
	}

	if m.stateFile != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.pollLoop(states)
		}()
	}

	last, have := LidOpen, false
	for {
		stopped := false
		select {
		case s := <-states:
			if have && s == last {
				continue
			}
			m.log.Printf("lid: state: %s", s)
			last, have = s, true
			m.callback(s)
		case <-m.stopCh:
			stopped = true
		}
		if stopped {
			break
		}
	}

	if eventFile != nil {
		eventFile.Close() // unblocks readEvdev
	}
	wg.Wait()
}

// pollLoop reads the state file immediately and then every 500 ms.
// Besides providing the initial state, it is the backstop for the evdev
// reader: if a transition is missed, the next poll corrects it.
func (m *Monitor) pollLoop(states chan<- LidState) {
	read := func() {
		s, err := readLidStateFromFile(m.stateFile, m.log, m.debug)
		if err != nil {
			m.log.Printf("lid: error reading state: %v", err)
			return
		}
		states <- s
	}
	read()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			read()
		}
	}
}

// readEvdev consumes input events from the lid switch's evdev device and
// converts SW_LID events into lid states (1 = open, 0 = closed).
// It exits on any read error; the polling backstop keeps the monitor
// alive.
func (m *Monitor) readEvdev(f *os.File, states chan<- LidState) {
	buf := make([]byte, inputEventSize)
	for {
		// A successful read on an evdev fd yields exactly one full
		// 16-byte input_event.
		if _, err := f.Read(buf); err != nil {
			m.log.Printf("lid: evdev read error: %v", err)
			return
		}
		typ := binary.LittleEndian.Uint16(buf[8:10])
		code := binary.LittleEndian.Uint16(buf[10:12])
		val := binary.LittleEndian.Uint32(buf[12:16])
		if m.debug {
			m.log.Printf("lid: evdev event type=%d code=0x%x value=%d", typ, code, val)
		}
		if typ != evSwitch || code != swLid {
			continue
		}
		if val == 1 {
			states <- LidOpen
		} else {
			states <- LidClosed
		}
	}
}

// Stop stops the monitor.
func (m *Monitor) Stop() {
	select {
	case <-m.stopCh:
		return
	default:
	}
	close(m.stopCh)
	<-m.done
}

// findLidSources locates the lid switch on disk.
//
//   - eventDevice: /dev/input/eventN whose /sys/class/input class entry
//     is named "Lid Switch". Consuming its event stream is the primary,
//     zero-latency path.
//   - stateFile: a file exposing the current state, used for the initial
//     state and the polling backstop. Checked in order:
//     /proc/acpi/button/lid/*/state ("open"/"closed"), then
//     /sys/class/input/*/device/lid_switch (0/1).
func findLidSources(log Logger, sysRoot, devRoot string) (eventDevice, stateFile string) {
	entries, _ := filepath.Glob(filepath.Join(sysRoot, "sys/class/input/*"))
	for _, entry := range entries {
		name, err := os.ReadFile(filepath.Join(entry, "name"))
		if err != nil || !strings.Contains(strings.ToLower(string(name)), "lid switch") {
			continue
		}
		base := filepath.Base(entry)
		if !strings.HasPrefix(base, "event") {
			continue
		}
		eventDevice = filepath.Join(devRoot, "input", base)
		log.Printf("lid: evdev device %s", eventDevice)
		break
	}

	for _, pattern := range []string{
		filepath.Join(sysRoot, "proc/acpi/button/lid/*/state"),
		filepath.Join(sysRoot, "sys/class/input/*/device/lid_switch"),
	} {
		matches, _ := filepath.Glob(pattern)
		if len(matches) > 0 {
			stateFile = matches[0]
			log.Printf("lid: state file %s", stateFile)
			break
		}
	}
	return
}

// readLidStateFromFile parses a lid state file. Supported formats:
//
//	"Lid switch is open" / "state:      open" / "closed"  (ACPI / proc)
//	"1" / "0"                                              (sysfs lid_switch)
func readLidStateFromFile(path string, log Logger, debug bool) (LidState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	content := strings.ToLower(strings.TrimSpace(string(data)))
	if debug {
		log.Printf("lid: read %q", content)
	}
	switch content {
	case "1":
		return LidOpen, nil
	case "0":
		return LidClosed, nil
	}
	if strings.Contains(content, "open") {
		return LidOpen, nil
	}
	if strings.Contains(content, "close") {
		return LidClosed, nil
	}
	return 0, fmt.Errorf("unknown lid state: %q", content)
}
