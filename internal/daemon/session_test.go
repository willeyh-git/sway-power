package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These tests mutate the process environment via t.Setenv and therefore
// cannot be parallelized (t.Setenv would panic on a parallel test).

// fakeBins creates an executable placeholder for each name in dir.
func fakeBins(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, name := range names {
		bin := filepath.Join(dir, name)
		if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
			t.Fatalf("write %s: %v", bin, err)
		}
	}
}

func TestSessionWarnings_CleanSession(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	fakeBins(t, dir, "swaylock", "swaymsg", "systemctl")
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Setenv("XDG_SESSION_TYPE", "wayland")

	if warnings := SessionWarnings(); len(warnings) != 0 {
		t.Fatalf("expected no warnings, got: %v", warnings)
	}
}

func TestSessionWarnings_BrokenSession(t *testing.T) {
	// Empty PATH dir: none of the binaries are found.
	t.Setenv("PATH", t.TempDir())
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("XDG_SESSION_TYPE", "x11")

	warnings := SessionWarnings()
	for _, want := range []string{
		"WAYLAND_DISPLAY",
		"XDG_RUNTIME_DIR",
		"XDG_SESSION_TYPE",
		"swaylock",
		"swaymsg",
		"systemctl",
	} {
		found := false
		for _, w := range warnings {
			if strings.Contains(w, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected a warning mentioning %q, got: %v", want, warnings)
		}
	}
}

func TestSessionWarnings_NoSessionTypeIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	fakeBins(t, dir, "swaylock", "swaymsg", "systemctl")
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Setenv("XDG_SESSION_TYPE", "")

	if warnings := SessionWarnings(); len(warnings) != 0 {
		t.Fatalf("expected no warnings, got: %v", warnings)
	}
}
