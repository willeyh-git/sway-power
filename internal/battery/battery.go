package battery

import (
	"fmt"
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

		capacityData, err := os.ReadFile(filepath.Join(path, "capacity"))
		if err != nil {
			return nil, fmt.Errorf("read battery capacity: %w", err)
		}

		percentage, err := strconv.Atoi(strings.TrimSpace(string(capacityData)))
		if err != nil {
			return nil, fmt.Errorf("parse battery capacity: %w", err)
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
