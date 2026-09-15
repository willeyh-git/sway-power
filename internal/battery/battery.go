package battery

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Status string

const (
	StatusCharging    Status = "charging"
	StatusDischarging Status = "discharging"
	StatusFull        Status = "full"
	StatusNotCharging Status = "not-charging"
	StatusUnknown     Status = "unknown"
)

type Battery struct {
	Name       string
	Percentage int
	Status     Status
}

func Read() (*Battery, error) {
	const powerSupplyPath = "/sys/class/power_supply"

	entries, err := os.ReadDir(powerSupplyPath)
	if err != nil {
		return nil, fmt.Errorf("read power supply directory: %w", err)
	}

	for _, entry := range entries {
		path := filepath.Join(powerSupplyPath, entry.Name())

		typeData, err := os.ReadFile(filepath.Join(path, "type"))
		if err != nil {
			continue
		}

		if strings.TrimSpace(string(typeData)) != "Battery" {
			continue
		}

		percentage, err := readBatteryPercentage(path)
		if err != nil {
			return nil, err
		}

		statusData, err := os.ReadFile(filepath.Join(path, "status"))
		if err != nil {
			return nil, fmt.Errorf("read battery status: %w", err)
		}

		return &Battery{
			Name:       entry.Name(),
			Percentage: percentage,
			Status:     parseStatus(strings.TrimSpace(string(statusData))),
		}, nil
	}

	return nil, nil
}

// readBatteryPercentage calculates the battery percentage from charge_now / charge_full
// (or energy_now / energy_full), falling back to the capacity file.
func readBatteryPercentage(path string) (int, error) {
	// Try charge_now / charge_full first.
	if pct, err := readRatio(path, "charge_now", "charge_full"); err == nil && pct > 0 {
		return pct, nil
	}

	// Try energy_now / energy_full.
	if pct, err := readRatio(path, "energy_now", "energy_full"); err == nil && pct > 0 {
		return pct, nil
	}

	// Fall back to capacity.
	data, err := os.ReadFile(filepath.Join(path, "capacity"))
	if err != nil {
		return 0, fmt.Errorf("read battery capacity: %w", err)
	}
	pct, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("parse battery capacity: %w", err)
	}
	return pct, nil
}

// readRatio reads two numeric files and returns file1 / file2 * 100 as an integer.
func readRatio(path, numFile, denFile string) (int, error) {
	numRaw, err := os.ReadFile(filepath.Join(path, numFile))
	if err != nil {
		return 0, err
	}
	denRaw, err := os.ReadFile(filepath.Join(path, denFile))
	if err != nil {
		return 0, err
	}
	num, err := strconv.ParseFloat(strings.TrimSpace(string(numRaw)), 64)
	if err != nil {
		return 0, err
	}
	den, err := strconv.ParseFloat(strings.TrimSpace(string(denRaw)), 64)
	if err != nil {
		return 0, err
	}
	if den <= 0 {
		return 0, fmt.Errorf("denominator %s is %f", denFile, den)
	}
	return int(math.Round(num / den * 100)), nil
}

func parseStatus(status string) Status {
	switch strings.ToLower(status) {
	case "charging":
		return StatusCharging
	case "discharging":
		return StatusDischarging
	case "full":
		return StatusFull
	case "not charging":
		return StatusNotCharging
	default:
		return StatusUnknown
	}
}
