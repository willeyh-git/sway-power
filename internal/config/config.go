package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Colors Colors `yaml:"colors"`
}

type Colors struct {
	Track    string `yaml:"track"`
	Normal   string `yaml:"normal"`
	Charging string `yaml:"charging"`
	Warning  string `yaml:"warning"`
	Critical string `yaml:"critical"`
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
}
