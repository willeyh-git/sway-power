// Package battery reads battery status from /sys/class/power_supply.
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
	TimeLeft   time.Duration // time to empty (discharging) or time to full (charging), 0 if unknown
	SizeWh     float64       // battery size in Wh, 0 if unknown
	Cycles     int           // charge cycles, 0 if unknown
	RateW      float64       // current charging/discharging power in W, 0 if unknown
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
		status := parseStatus(strings.TrimSpace(string(statusData)))

		return &Battery{
			Name:       entry.Name(),
			Percentage: percentage,
			Status:     status,
			TimeLeft:   readTimeLeft(path, status),
			SizeWh:     readSizeWh(path),
			Cycles:     readCycles(path),
			RateW:      readRateW(path),
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

// readTimeLeft estimates time to empty (discharging) or time to full
// (charging) from the remaining charge divided by the current average.
// Returns 0 if the values can't be read.
func readTimeLeft(path string, status Status) time.Duration {
	// Remaining charge to run on, in µAh.
	var remaining float64
	switch status {
	case StatusDischarging:
		v, ok := readFloatFile(path, "charge_now")
		if !ok {
			return 0
		}
		remaining = v
	case StatusCharging:
		full, okFull := readFloatFile(path, "charge_full")
		now, okNow := readFloatFile(path, "charge_now")
		if !okFull || !okNow {
			return 0
		}
		remaining = full - now
	default:
		return 0
	}
	if remaining <= 0 {
		return 0
	}

	// current_avg first, current_now as a more volatile fallback.
	if current, ok := readFloatFile(path, "current_avg"); ok && current != 0 {
		return chargeDuration(remaining, abs(current))
	}
	if current, ok := readFloatFile(path, "current_now"); ok && current != 0 {
		return chargeDuration(remaining, abs(current))
	}
	return 0
}

// chargeDuration converts a charge (µAh) and a current (µA) into a duration.
func chargeDuration(charge, current float64) time.Duration {
	if charge <= 0 || current <= 0 {
		return 0
	}
	hours := charge / current
	return time.Duration(hours * float64(time.Hour))
}

// readSizeWh reads the battery size in Wh.
// Prefers energy_full (1/10 Wh units), falls back to
// charge_full (µAh) × voltage_now.
func readSizeWh(path string) float64 {
	if energy, ok := readFloatFile(path, "energy_full"); ok {
		return energy / 10
	}
	chargeFull, ok := readFloatFile(path, "charge_full")
	if !ok {
		return 0
	}
	voltage, ok := readVoltageV(path)
	if !ok {
		return 0
	}
	// Ah × V = Wh
	return (chargeFull / 1e6) * voltage
}

// readVoltageV reads voltage_now in volts. sysfs documents the file as
// millivolts, but some drivers report microvolts; normalize with a sanity
// check (a value over 25000 can't be mV for a laptop battery).
func readVoltageV(path string) (float64, bool) {
	v, ok := readFloatFile(path, "voltage_now")
	if !ok {
		return 0, false
	}
	if v > 25000 {
		v /= 1000 // µV → mV
	}
	if v <= 0 {
		return 0, false
	}
	return v / 1000, true // mV → V
}

// readCycles reads the number of charge cycles from cycle_count.
func readCycles(path string) int {
	if cycles, ok := readFloatFile(path, "cycle_count"); ok {
		return int(math.Round(cycles))
	}
	return 0
}

// readRateW reads the current charging/discharging power in W.
// Prefers power_now (µW), falls back to current_avg (µA) × voltage_now.
func readRateW(path string) float64 {
	if power, ok := readFloatFile(path, "power_now"); ok && power != 0 {
		return abs(power) / 1e6
	}
	current, ok := readFloatFile(path, "current_avg")
	if !ok {
		current, ok = readFloatFile(path, "current_now")
	}
	if !ok {
		return 0
	}
	voltage, ok := readVoltageV(path)
	if !ok {
		return 0
	}
	// A × V = W
	return (abs(current) / 1e6) * voltage
}

// readFloatFile reads a numeric sysfs file and returns its value.
func readFloatFile(path, name string) (float64, bool) {
	data, err := os.ReadFile(filepath.Join(path, name))
	if err != nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
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
