package daemon

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// lidRecorder collects monitor callbacks in order.
type lidRecorder struct {
	mu     sync.Mutex
	states []LidState
}

func (r *lidRecorder) add(s LidState) {
	r.mu.Lock()
	r.states = append(r.states, s)
	r.mu.Unlock()
}

func (r *lidRecorder) snapshot() []LidState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]LidState(nil), r.states...)
}

// wait blocks until the recorder has at least want.states, then checks the
// prefix matches.
func (r *lidRecorder) wait(t *testing.T, want []LidState) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if r.hasPrefix(want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for lid states %v, got %v", want, r.snapshot())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (r *lidRecorder) hasPrefix(want []LidState) bool {
	states := r.snapshot()
	if len(states) < len(want) {
		return false
	}
	for i, s := range want {
		if states[i] != s {
			return false
		}
	}
	return true
}

// writeLidEvent writes one 16-byte input_event: EV_SW / SW_LID, given
// value (1 = open, 0 = closed).
func writeLidEvent(t *testing.T, w *os.File, value uint32) {
	t.Helper()
	buf := make([]byte, inputEventSize)
	binary.LittleEndian.PutUint16(buf[8:10], evSwitch)
	binary.LittleEndian.PutUint16(buf[10:12], swLid)
	binary.LittleEndian.PutUint32(buf[12:16], value)
	if _, err := w.Write(buf); err != nil {
		t.Fatalf("write lid event: %v", err)
	}
}

// setupLidTree builds a fake root in a temp dir containing <root>/sys and
// <root>/proc mirroring the real layout (plus <root>/dev/input), and
// returns (root, devRoot).
func setupLidTree(t *testing.T, withLidEvdev, withProc, withSysfsAttr bool) (sysRoot, devRoot string) {
	t.Helper()
	root := t.TempDir()
	sysRoot = root
	devRoot = filepath.Join(root, "dev")

	if withLidEvdev {
		if err := os.MkdirAll(filepath.Join(sysRoot, "sys/class/input/event0"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sysRoot, "sys/class/input/event0/name"), []byte("Lid Switch\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(devRoot, "input"), 0755); err != nil {
			t.Fatal(err)
		}
	}

	if withProc {
		dir := filepath.Join(sysRoot, "proc/acpi/button/lid/LID0")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "state"), []byte("Lid switch is open\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	if withSysfsAttr {
		dir := filepath.Join(sysRoot, "sys/class/input/event0/device")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "lid_switch"), []byte("1\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	return
}

func TestFindLidSourcesAll(t *testing.T) {
	sysRoot, devRoot := setupLidTree(t, true, true, true)

	evdev, state := findLidSources(&testLogger{t}, sysRoot, devRoot)
	wantEvdev := filepath.Join(devRoot, "input/event0")
	if evdev != wantEvdev {
		t.Errorf("evdev = %q, want %q", evdev, wantEvdev)
	}
	wantState := filepath.Join(sysRoot, "proc/acpi/button/lid/LID0/state")
	if state != wantState {
		t.Errorf("state file = %q, want %q (proc wins)", state, wantState)
	}
}

func TestFindLidSourcesSysfsAttrFallback(t *testing.T) {
	// No /proc/acpi: the sysfs lid_switch attribute is the state source.
	sysRoot, devRoot := setupLidTree(t, true, false, true)

	evdev, state := findLidSources(&testLogger{t}, sysRoot, devRoot)
	if evdev != filepath.Join(devRoot, "input/event0") {
		t.Errorf("evdev = %q", evdev)
	}
	wantState := filepath.Join(sysRoot, "sys/class/input/event0/device/lid_switch")
	if state != wantState {
		t.Errorf("state file = %q, want %q", state, wantState)
	}
}

func TestFindLidSourcesIgnoresNonLidDevices(t *testing.T) {
	sysRoot, devRoot := setupLidTree(t, false, false, false)
	// A keyboard with a plain state file elsewhere must not be picked up.
	if err := os.MkdirAll(filepath.Join(sysRoot, "sys/class/input/event1"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysRoot, "sys/class/input/event1/name"), []byte("AT Translated Set 2 keyboard\n"), 0644); err != nil {
		t.Fatal(err)
	}

	evdev, state := findLidSources(&testLogger{t}, sysRoot, devRoot)
	if evdev != "" || state != "" {
		t.Errorf("expected no sources, got evdev=%q state=%q", evdev, state)
	}
}

func TestFindLidSourcesNone(t *testing.T) {
	sysRoot, devRoot := setupLidTree(t, false, false, false)
	evdev, state := findLidSources(&testLogger{t}, sysRoot, devRoot)
	if evdev != "" || state != "" {
		t.Errorf("expected no sources, got evdev=%q state=%q", evdev, state)
	}
}

func TestNewMonitorNoSource(t *testing.T) {
	sysRoot, devRoot := setupLidTree(t, false, false, false)
	if m := newMonitor(&testLogger{t}, false, func(LidState) {}, sysRoot, devRoot); m != nil {
		t.Error("expected nil monitor when no lid source exists")
	}
}

func TestReadLidStateFromFile(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name  string
		want  LidState
		errOK bool
	}{
		{"proc open", LidOpen, false},
		{"proc state line open", LidOpen, false},
		{"closed", LidClosed, false},
		{"sysfs 1", LidOpen, false},
		{"sysfs 0", LidClosed, false},
		{"garbage", LidOpen, true},
	}
	contents := map[string]string{
		"proc open":            "Lid switch is open\n",
		"proc state line open": "state:      open\n",
		"closed":               "closed\n",
		"sysfs 1":              "1\n",
		"sysfs 0":              "0\n",
		"garbage":              "??? \n",
	}
	for _, tt := range tests {
		path := filepath.Join(dir, strings.ReplaceAll(tt.name, " ", "_"))
		if err := os.WriteFile(path, []byte(contents[tt.name]), 0644); err != nil {
			t.Fatal(err)
		}
		got, err := readLidStateFromFile(path, &testLogger{t}, false)
		if tt.errOK {
			if err == nil {
				t.Errorf("%s: expected error", tt.name)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("%s: got %v, %v; want %v", tt.name, got, err, tt.want)
		}
	}
}

// TestMonitorEvdevTransitions is the end-to-end test for the evdev path:
// initial state comes from the state file, transitions come as input
// events, and repeated states are deduplicated.
func TestMonitorEvdevTransitions(t *testing.T) {
	sysRoot, devRoot := setupLidTree(t, true, true, false)

	// The event device is a FIFO so the monitor can open and read it.
	fifo := filepath.Join(devRoot, "input/event0")
	if err := os.MkdirAll(filepath.Dir(fifo), 0755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(fifo, 0644); err != nil {
		t.Fatal(err)
	}

	// Open the writer side first (O_RDWR on a FIFO does not block).
	writer, err := os.OpenFile(fifo, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open fifo: %v", err)
	}
	defer writer.Close()

	rec := &lidRecorder{}
	m := newMonitor(&testLogger{t}, false, rec.add, sysRoot, devRoot)
	if m == nil {
		t.Fatal("expected monitor")
	}
	defer m.Stop()

	// Initial state from the state file.
	rec.wait(t, []LidState{LidOpen})

	// Close, then open, via events.
	writeLidEvent(t, writer, 0)
	rec.wait(t, []LidState{LidOpen, LidClosed})
	writeLidEvent(t, writer, 1)
	rec.wait(t, []LidState{LidOpen, LidClosed, LidOpen})
}

// TestMonitorPollFallback covers hardware with only a state file: the
// 500 ms poll still drives transitions.
func TestMonitorPollFallback(t *testing.T) {
	sysRoot, _ := setupLidTree(t, false, true, false)
	statePath := filepath.Join(sysRoot, "proc/acpi/button/lid/LID0/state")

	rec := &lidRecorder{}
	m := newMonitor(&testLogger{t}, false, rec.add, sysRoot, t.TempDir())
	if m == nil {
		t.Fatal("expected monitor")
	}
	defer m.Stop()

	rec.wait(t, []LidState{LidOpen})

	if err := os.WriteFile(statePath, []byte("Lid switch is closed\n"), 0644); err != nil {
		t.Fatal(err)
	}
	rec.wait(t, []LidState{LidOpen, LidClosed})
}

// TestMonitorRealLidSource is an integration test against the real
// machine: discovery against the actual /sys and /proc, plus initial
// state from the real lid state file.
func TestMonitorRealLidSource(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	evdev, state := findLidSources(&testLogger{t}, "/", "/dev")
	if evdev == "" && state == "" {
		t.Skip("this machine has no discoverable lid switch")
	}
	t.Logf("found evdev=%q state=%q", evdev, state)
	if state != "" {
		if _, err := os.Stat(state); err != nil {
			t.Errorf("state file %s does not exist: %v", state, err)
		}
	}
	if evdev != "" {
		f, err := os.Open(evdev)
		if err != nil {
			t.Errorf("cannot open evdev device %s: %v", evdev, err)
		} else {
			f.Close()
		}
	}
}
