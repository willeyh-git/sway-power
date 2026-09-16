// Package config loads and validates sway-power configuration.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	UI       UIColors `yaml:"ui"`
	LidClose LidClose `yaml:"lid_close"`
}

type LidClose struct {
	Action string `yaml:"action"`
}

// UIColors holds user-overridable theme colors. Every field is optional:
// an empty value means "fall back to the default palette" (see ui.BuildPalette).
// A user-provided value always takes precedence, so e.g. overriding only
// ui.accent leaves everything else on its fallback.
type UIColors struct {
	Mode         string `yaml:"mode"`          // auto | light | dark
	Accent       string `yaml:"accent"`        // blue 3 by default; also drives icon, borders, hover, selected, track fill
	Background   string `yaml:"background"`    // window background
	Track        string `yaml:"track"`         // battery track background (a different shade of background)
	Label        string `yaml:"label"`         // muted text: grid labels, status line
	Value        string `yaml:"value"`         // emphasized text: grid values, percentage
	Category     string `yaml:"category"`      // section headings: "Power Profile", "Lid Settings"
	Title        string `yaml:"title"`         // strongest text: "Battery" heading
	Icon         string `yaml:"icon"`          // battery icon (default: accent)
	Button       string `yaml:"button"`        // unselected button background
	ButtonLabel  string `yaml:"button_label"`  // unselected button text
	ButtonBorder string `yaml:"button_border"` // selected button border (default: accent)
	ButtonHover  string `yaml:"button_hover"`  // button hover/focus background (default: accent)
	ButtonActive string `yaml:"button_active"` // selected button background (default: accent)
}

// Validate checks that the lid close action is valid.
func (l LidClose) Validate() error {
	if l.Action == "" {
		return nil // empty is OK — means "don't handle it"
	}
	switch l.Action {
	case "lock", "sleep", "nothing":
		return nil
	default:
		return fmt.Errorf("lid_close.action: invalid action %q (valid: lock, sleep, nothing)", l.Action)
	}
}

// Validate checks that the ui section is syntactically valid.
// Empty values are treated as "unset" and fall back to the defaults.
func (u UIColors) Validate() error {
	mode := strings.ToLower(strings.TrimSpace(u.Mode))
	switch mode {
	case "", "auto", "light", "dark":
	default:
		return fmt.Errorf("ui.mode: invalid mode %q (valid: auto, light, dark)", u.Mode)
	}

	fields := []struct {
		key   string
		value string
	}{
		{"accent", u.Accent},
		{"background", u.Background},
		{"track", u.Track},
		{"label", u.Label},
		{"value", u.Value},
		{"category", u.Category},
		{"title", u.Title},
		{"icon", u.Icon},
		{"button", u.Button},
		{"button_label", u.ButtonLabel},
		{"button_border", u.ButtonBorder},
		{"button_hover", u.ButtonHover},
		{"button_active", u.ButtonActive},
	}

	for _, f := range fields {
		if f.value == "" {
			continue
		}

		if !isValidHexColor(f.value) {
			return fmt.Errorf("ui.%s: invalid color %q", f.key, f.value)
		}
	}

	return nil
}

// Validate checks all config values that can be wrong without being
// syntactically invalid YAML.
func (c Config) Validate() error {
	if err := c.UI.Validate(); err != nil {
		return err
	}
	return c.LidClose.Validate()
}

// isValidHexColor reports whether value is a 6-digit hex color, optionally
// prefixed with '#'. This is the format the UI renders; anything else is
// rejected so a typo fails at startup instead of turning the UI gray.
func isValidHexColor(value string) bool {
	value = strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(value) != 6 {
		return false
	}

	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}

	return true
}

func Default() Config {
	return Config{
		UI: UIColors{
			Mode: "auto", // follow the system theme
		},
		LidClose: LidClose{
			Action: "lock", // default: lock screen when lid closes
		},
	}
}

func Load() (Config, error) {
	config := Default()

	configDir, err := os.UserConfigDir()
	if err != nil {
		return config, fmt.Errorf("get config directory: %w", err)
	}

	path := filepath.Join(
		configDir,
		"sway-power",
		"config.yaml",
	)

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return config, nil
		}

		return config, fmt.Errorf("read config: %w", err)
	}

	var fileConfig Config

	if err := yaml.Unmarshal(data, &fileConfig); err != nil {
		return config, fmt.Errorf("parse config: %w", err)
	}

	merge(&config, fileConfig)

	if err := config.Validate(); err != nil {
		return config, fmt.Errorf("config: %w", err)
	}

	return config, nil
}

func merge(dst *Config, src Config) {
	if src.UI.Mode != "" {
		dst.UI.Mode = src.UI.Mode
	}

	if src.UI.Accent != "" {
		dst.UI.Accent = src.UI.Accent
	}

	if src.UI.Background != "" {
		dst.UI.Background = src.UI.Background
	}

	if src.UI.Track != "" {
		dst.UI.Track = src.UI.Track
	}

	if src.UI.Label != "" {
		dst.UI.Label = src.UI.Label
	}

	if src.UI.Value != "" {
		dst.UI.Value = src.UI.Value
	}

	if src.UI.Category != "" {
		dst.UI.Category = src.UI.Category
	}

	if src.UI.Title != "" {
		dst.UI.Title = src.UI.Title
	}

	if src.UI.Icon != "" {
		dst.UI.Icon = src.UI.Icon
	}

	if src.UI.Button != "" {
		dst.UI.Button = src.UI.Button
	}

	if src.UI.ButtonLabel != "" {
		dst.UI.ButtonLabel = src.UI.ButtonLabel
	}

	if src.UI.ButtonBorder != "" {
		dst.UI.ButtonBorder = src.UI.ButtonBorder
	}

	if src.UI.ButtonHover != "" {
		dst.UI.ButtonHover = src.UI.ButtonHover
	}

	if src.UI.ButtonActive != "" {
		dst.UI.ButtonActive = src.UI.ButtonActive
	}

	if src.LidClose.Action != "" {
		dst.LidClose.Action = src.LidClose.Action
	}
}
