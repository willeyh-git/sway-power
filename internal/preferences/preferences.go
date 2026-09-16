// Package preferences persists user preferences to disk.
package preferences

import (
	"encoding/json"
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
func Load() (Preferences, error) {
	prefs := Default()

	configDir, err := os.UserConfigDir()
	if err != nil {
		return prefs, nil
	}

	path := filepath.Join(configDir, "sway-power", "preferences.json")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return prefs, nil
		}
		return prefs, nil // silently ignore read errors
	}

	if err := json.Unmarshal(data, &prefs); err != nil {
		return prefs, nil // silently ignore parse errors
	}

	return prefs, nil
}

// Save writes preferences to the user config directory.
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

	return os.WriteFile(filepath.Join(dir, "preferences.json"), data, 0644)
}
