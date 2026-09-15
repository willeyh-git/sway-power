package ui

import (
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2/widget"

	"github.com/willeyh-git/sway-power/internal/battery"
)

// emaSmooth applies exponential moving average smoothing to a time.Duration.
// alpha=0.2 gives a responsive but stable estimate.
type emaSmooth struct {
	value time.Duration
	alpha float64
}

func newEMASmooth(alpha float64) *emaSmooth {
	return &emaSmooth{alpha: alpha}
}

func (e *emaSmooth) reset() {
	e.value = 0
}

func (e *emaSmooth) update(newVal time.Duration) time.Duration {
	if e.value == 0 {
		e.value = newVal
		return newVal
	}
	// EMA: new = alpha * new + (1 - alpha) * old
	n := float64(newVal)
	o := float64(e.value)
	e.value = time.Duration(e.alpha*n + (1-e.alpha)*o)
	return e.value
}

// setTextable is an interface for widgets that can have their text set.
type setTextable interface {
	SetText(string)
}

// batteryDisplay handles battery percentage, status, and time-left display.
type batteryDisplay struct {
	percentage *widget.Label
	status     setTextable
	detail     setTextable
	widget     *BatteryWidget
	ema        *emaSmooth
	lastStatus battery.Status
}

func newBatteryDisplay(
	percentage *widget.Label,
	status setTextable,
	detail setTextable,
	batWidget *BatteryWidget,
) *batteryDisplay {
	return &batteryDisplay{
		percentage: percentage,
		status:     status,
		detail:     detail,
		widget:     batWidget,
		ema:        newEMASmooth(0.2), // 20% weight on new reading
	}
}

func (d *batteryDisplay) update(bat *battery.Battery, err error) {
	if err != nil {
		d.percentage.SetText("Error")
		d.status.SetText(err.Error())
		d.detail.SetText("")
		d.widget.SetBattery(nil)
		return
	}

	if bat == nil {
		d.percentage.SetText("No battery")
		d.status.SetText("")
		d.detail.SetText("")
		d.widget.SetBattery(nil)
		return
	}

	d.percentage.SetText(fmt.Sprintf("%d%%", bat.Percentage))
	d.widget.SetBattery(bat)
	d.detail.SetText(batteryDetails(bat))

	// Smooth the time estimate, but never blend estimates from a
	// different status (time-to-full vs time-to-empty).
	if bat.Status != d.lastStatus {
		d.ema.reset()
		d.lastStatus = bat.Status
	}
	if bat.TimeLeft > 0 {
		smoothed := d.ema.update(bat.TimeLeft)
		d.status.SetText(fmt.Sprintf("%s · %s", bat.Status, formatDuration(smoothed)))
	} else {
		d.status.SetText(string(bat.Status))
	}
}

// batteryDetails formats the extra battery info: size in Wh, charge cycles,
// and current charge/discharge power in W. Unknown values are omitted.
func batteryDetails(bat *battery.Battery) string {
	parts := make([]string, 0, 3)
	if bat.SizeWh > 0 {
		parts = append(parts, fmt.Sprintf("%.1f Wh", bat.SizeWh))
	}
	if bat.Cycles > 0 {
		parts = append(parts, fmt.Sprintf("%d cycles", bat.Cycles))
	}
	if bat.RateW > 0 {
		if bat.Status == battery.StatusCharging {
			parts = append(parts, fmt.Sprintf("charging at %.1f W", bat.RateW))
		} else {
			parts = append(parts, fmt.Sprintf("discharging at %.1f W", bat.RateW))
		}
	}
	return strings.Join(parts, " · ")
}
