package battery

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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
	TimeLeft   time.Duration // estimated time remaining, 0 if unknown
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

		timeLeft := readTimeLeft(path)

		statusData, err := os.ReadFile(filepath.Join(path, "status"))
		if err != nil {
			return nil, fmt.Errorf("read battery status: %w", err)
		}

		return &Battery{
			Name:       entry.Name(),
			Percentage: percentage,
			Status:     parseStatus(strings.TrimSpace(string(statusData))),
			TimeLeft:   timeLeft,
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

// readTimeLeft estimates remaining battery time in hours.
// Uses charge_now / current_avg for charge-based batteries.
// Returns 0 if the values can't be read or the battery is charging/full.
func readTimeLeft(path string) time.Duration {
	// Only estimate when discharging.
	status, err := os.ReadFile(filepath.Join(path, "status"))
	if err != nil {
		return 0
	}
	if strings.TrimSpace(strings.ToLower(string(status))) != "discharging" {
		return 0
	}

	// Try charge_now / current_avg first.
	chargeRaw, err := os.ReadFile(filepath.Join(path, "charge_now"))
	if err == nil {
		avgRaw, err := os.ReadFile(filepath.Join(path, "current_avg"))
		if err == nil {
			charge, err := strconv.ParseFloat(strings.TrimSpace(string(chargeRaw)), 64)
			avg, err := strconv.ParseFloat(strings.TrimSpace(string(avgRaw)), 64)
			if err == nil && avg > 0 {
				// charge_now is in µAh, current_avg in µA → result in hours
				hours := charge / avg
				return time.Duration(hours * float64(time.Hour))
			}
		}
	}

	// Fallback: charge_now / current_now (more volatile).
	currRaw, err := os.ReadFile(filepath.Join(path, "current_now"))
	if err != nil {
		return 0
	}
	curr, err := strconv.ParseFloat(strings.TrimSpace(string(currRaw)), 64)
	if err != nil || curr <= 0 {
		return 0
	}
	charge, err := strconv.ParseFloat(strings.TrimSpace(string(chargeRaw)), 64)
	if err != nil {
		return 0
	}
	hours := charge / curr
	return time.Duration(hours * float64(time.Hour))
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
