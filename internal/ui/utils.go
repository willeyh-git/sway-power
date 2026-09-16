// Package ui builds the fyne user interface for the power widget.
package ui

import (
	"encoding/json"
	"fmt"
	"image/color"
	"os/exec"
	"strconv"
	"strings"
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

// parseHexColor parses a hex color string (with or without # prefix) into a color.NRGBA.
func parseHexColor(hex string) color.NRGBA {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xff}
	}

	r, err := strconv.ParseInt(hex[0:2], 16, 64)
	if err != nil {
		return color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xff}
	}
	g, err := strconv.ParseInt(hex[2:4], 16, 64)
	if err != nil {
		return color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xff}
	}
	b, err := strconv.ParseInt(hex[4:6], 16, 64)
	if err != nil {
		return color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xff}
	}

	return color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 0xff}
}
