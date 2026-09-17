package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newFixture wires the package's systemctl entry points to recorders and
// points XDG_CONFIG_HOME / XDG_RUNTIME_DIR at a temp dir, so Bootstrap /
// Uninstall exercise the real unit-file logic against a fake user
// manager. No real systemctl is ever invoked.
type fixture struct {
	log    []string // systemctl invocations, in order
	active bool     // what isActive reports
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_RUNTIME_DIR", dir)

	oldSys, oldActive, oldImport := systemctl, isActive, importEnvironment
	f := &fixture{}
	systemctl = func(args ...string) error {
		f.log = append(f.log, strings.Join(args, " "))
		return nil
	}
	isActive = func(string) bool { return f.active }
	importEnvironment = func() error {
		f.log = append(f.log, "import-environment")
		return nil
	}
	t.Cleanup(func() {
		systemctl, isActive, importEnvironment = oldSys, oldActive, oldImport
	})
	return f
}

func writeUnit(t *testing.T, content string) string {
	t.Helper()
	p, err := UnitPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func unitFor(execPath string) string {
	return strings.Replace(unitTemplate, "__EXEC_PATH__", execPath, 1)
}

func wantCalls(t *testing.T, f *fixture, want ...string) {
	t.Helper()
	if len(f.log) != len(want) {
		t.Fatalf("systemctl calls: got %v, want %v", f.log, want)
	}
	for i, w := range want {
		if f.log[i] != w {
			t.Fatalf("systemctl call %d: got %q, want %q (all: %v)", i, f.log[i], w, f.log)
		}
	}
}

// The in-place upgrade: same ExecStart path, new binary. The running
// daemon must be restarted so it actually serves the new binary.
func TestBootstrapUnchangedRestartsRunningService(t *testing.T) {
	f := newFixture(t)
	path := "/usr/local/bin/sway-power"
	writeUnit(t, unitFor(path))
	f.active = true

	if err := Bootstrap(path); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	wantCalls(t, f, "restart "+unitName)
}

// A deliberately stopped daemon stays stopped: no unit rewrite, no
// restart, nothing.
func TestBootstrapUnchangedStoppedServiceStaysStopped(t *testing.T) {
	f := newFixture(t)
	path := "/usr/local/bin/sway-power"
	writeUnit(t, unitFor(path))
	f.active = false

	if err := Bootstrap(path); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	wantCalls(t, f)
}

func TestBootstrapFirstInstall(t *testing.T) {
	f := newFixture(t)
	path := "/opt/sway/bin/sway-power"

	if err := Bootstrap(path); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	wantCalls(t, f,
		"import-environment",
		"daemon-reload",
		"enable --now "+unitName,
	)

	p, _ := UnitPath()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != unitFor(path) {
		t.Fatalf("unit content mismatch:\ngot  %s\nwant %s", data, unitFor(path))
	}
}

func TestBootstrapUpgradeRestartsService(t *testing.T) {
	f := newFixture(t)
	old, new := "/opt/old/sway-power", "/usr/local/bin/sway-power"
	writeUnit(t, unitFor(old))
	f.active = true

	if err := Bootstrap(new); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	wantCalls(t, f,
		"import-environment",
		"daemon-reload",
		"enable "+unitName,
		"restart "+unitName,
	)

	p, _ := UnitPath()
	data, _ := os.ReadFile(p)
	if string(data) != unitFor(new) {
		t.Fatalf("unit not rewritten to new path")
	}
}

func TestUninstallStopsDisablesRemoves(t *testing.T) {
	f := newFixture(t)
	writeUnit(t, unitFor("/usr/local/bin/sway-power"))

	if err := Uninstall(); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	wantCalls(t, f,
		"disable --now "+unitName,
		"daemon-reload",
	)

	p, _ := UnitPath()
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("unit file should be removed")
	}

	// Idempotent: uninstalling again is a no-op.
	f.log = nil
	if err := Uninstall(); err != nil {
		t.Fatalf("second Uninstall: %v", err)
	}
	wantCalls(t, f)
}
