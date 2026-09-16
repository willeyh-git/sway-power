package preferences

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromPathValid(t *testing.T) {
	tmpDir := t.TempDir()
	prefsPath := filepath.Join(tmpDir, "preferences.json")

	if err := os.WriteFile(prefsPath, []byte(`{"lid_close": "sleep"}`), 0644); err != nil {
		t.Fatalf("setup: write prefs: %v", err)
	}

	prefs, err := LoadFromPath(prefsPath)
	if err != nil {
		t.Fatalf("LoadFromPath: %v", err)
	}
	if prefs.LidClose != "sleep" {
		t.Errorf("LidClose = %q, want %q", prefs.LidClose, "sleep")
	}
}

func TestLoadFromPathMissingReturnsDefaults(t *testing.T) {
	prefs, err := LoadFromPath(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err != nil {
		t.Fatalf("LoadFromPath(missing): %v", err)
	}
	if prefs != Default() {
		t.Errorf("LidClose = %q, want default %q", prefs.LidClose, Default().LidClose)
	}
}

func TestLoadFromPathCorruptJSONReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	prefsPath := filepath.Join(tmpDir, "preferences.json")

	// Unparseable JSON: this is what a half-written or hand-corrupted
	// file looks like. LoadFromPath must report it, so the daemon's
	// watcher keeps the last-good action instead of swapping to the
	// defaults.
	corrupt := `{"lid_close": "lock",`
	if err := os.WriteFile(prefsPath, []byte(corrupt), 0644); err != nil {
		t.Fatalf("setup: write corrupt prefs: %v", err)
	}

	_, err := LoadFromPath(prefsPath)
	if err == nil {
		t.Fatal("LoadFromPath(corrupt JSON): expected error, got nil")
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	// Save() writes to the real user config dir; redirect it.
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)

	prefs := Preferences{LidClose: "nothing"}
	if err := Save(prefs); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := LoadFromPath(defaultPath())
	if err != nil {
		t.Fatalf("LoadFromPath: %v", err)
	}
	if got != prefs {
		t.Errorf("roundtrip: got %+v, want %+v", got, prefs)
	}
}
