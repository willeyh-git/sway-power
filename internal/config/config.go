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
	Colors    Colors     `yaml:"colors"`
	LidClose  LidClose   `yaml:"lid_close"`
}

type LidClose struct {
	Action string `yaml:"action"`
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

type Colors struct {
	Track    string `yaml:"track"`
	Normal   string `yaml:"normal"`
	Charging string `yaml:"charging"`
	Warning  string `yaml:"warning"`
	Critical string `yaml:"critical"`
}

// Validate checks that every configured color is a valid #RRGGBB hex color.
// Empty values are treated as "unset" and fall back to the defaults.
func (c Colors) Validate() error {
	fields := []struct {
		key   string
		value string
	}{
		{"track", c.Track},
		{"normal", c.Normal},
		{"charging", c.Charging},
		{"warning", c.Warning},
		{"critical", c.Critical},
	}

	for _, f := range fields {
		if f.value == "" {
			continue
		}

		if !isValidHexColor(f.value) {
			return fmt.Errorf("colors.%s: invalid color %q", f.key, f.value)
		}
	}

	return nil
}

// Validate checks all config values that can be wrong without being
// syntactically invalid YAML.
func (c Config) Validate() error {
	if err := c.Colors.Validate(); err != nil {
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
		Colors: Colors{
			Track:    "#2d2d32",
			Normal:   "#50c878",
			Charging: "#50aaff",
			Warning:  "#f0b43c",
			Critical: "#e64646",
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
	if src.Colors.Track != "" {
		dst.Colors.Track = src.Colors.Track
	}

	if src.Colors.Normal != "" {
		dst.Colors.Normal = src.Colors.Normal
	}

	if src.Colors.Charging != "" {
		dst.Colors.Charging = src.Colors.Charging
	}

	if src.Colors.Warning != "" {
		dst.Colors.Warning = src.Colors.Warning
	}

	if src.Colors.Critical != "" {
		dst.Colors.Critical = src.Colors.Critical
	}

	if src.LidClose.Action != "" {
		dst.LidClose.Action = src.LidClose.Action
	}
}
