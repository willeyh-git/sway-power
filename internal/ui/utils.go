package ui

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// getWaylandScale reads the display scale from Sway/Wayland.
// Returns 0 if the scale cannot be determined.
func getWaylandScale() float64 {
	// Try swaymsg first.
	cmd := exec.Command("swaymsg", "-t", "get_outputs")
	out, err := cmd.Output()
	if err != nil {
		return 0
	}

	type output struct {
		Scale float64 `json:"scale"`
	}

	var outputs []output
	if err := json.Unmarshal(out, &outputs); err != nil {
		return 0
	}

	// Return the first non-zero scale.
	for _, o := range outputs {
		if o.Scale > 0 {
			return o.Scale
		}
	}

	return 0
}

// formatDuration formats a time.Duration as "n h n min".
func formatDuration(d time.Duration) string {
	totalMinutes := int(d.Round(time.Minute).Minutes())
	h := totalMinutes / 60
	m := totalMinutes % 60
	if h == 0 {
		return fmt.Sprintf("%d min", m)
	}
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh %dmin", h, m)
}
