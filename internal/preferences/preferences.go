// Package preferences persists user preferences to disk.
package preferences

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Preferences stores user-configurable settings.
type Preferences struct {
	LidClose string `json:"lid_close"` // lock, sleep, nothing
}

// Default returns the default preferences.
func Default() Preferences {
	return Preferences{
		LidClose: "lock",
	}
}

// Load reads preferences from the user config directory.
//
// Unlike LoadFromPath, read and parse errors are swallowed and the
// defaults are returned: the GUI degrades to defaults on a missing or
// corrupt file rather than refusing to start. The daemon uses
// LoadFromPath directly, because on a parse/validate failure it must
// keep its last-good action.
func Load() (Preferences, error) {
	prefs, err := LoadFromPath(defaultPath())
	if err != nil {
		return Default(), nil
	}
	return prefs, nil
}

// defaultPath returns the default preferences file path.
func defaultPath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(configDir, "sway-power", "preferences.json")
}

// LoadFromPath reads preferences from the given path.
//
// A corrupt (unparseable) or unreadable file is an error: callers such
// as the daemon's preferences watcher must be able to distinguish a
// real change from a transient failure and keep their last-good state
// instead of swapping to defaults. A missing file is not an error: it
// means "no preferences yet" and the defaults are returned.
func LoadFromPath(path string) (Preferences, error) {
	if path == "" {
		return Default(), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return Preferences{}, fmt.Errorf("read preferences %s: %w", path, err)
	}

	var prefs Preferences
	if err := json.Unmarshal(data, &prefs); err != nil {
		return Preferences{}, fmt.Errorf("parse preferences %s: %w", path, err)
	}

	return prefs, nil
}

// Save writes preferences to the user config directory atomically:
// write to a .tmp file, fsync (best effort), then rename over the
// final path. This ensures the polling daemon always sees complete files.
func Save(prefs Preferences) error {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}

	dir := filepath.Join(configDir, "sway-power")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(prefs, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(dir, "preferences.json")
	tmpPath := path + ".tmp"

	// Write to temp file.
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}

	// Best-effort fsync.
	if f, err := os.Open(tmpPath); err == nil {
		f.Sync()
		f.Close()
	}

	// Atomic rename.
	return os.Rename(tmpPath, path)
}
