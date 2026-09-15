package ui

import (
	"fmt"
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
	widget     *BatteryWidget
	ema        *emaSmooth
}

func newBatteryDisplay(
	percentage *widget.Label,
	status setTextable,
	batWidget *BatteryWidget,
) *batteryDisplay {
	return &batteryDisplay{
		percentage: percentage,
		status:     status,
		widget:     batWidget,
		ema:        newEMASmooth(0.2), // 20% weight on new reading
	}
}

func (d *batteryDisplay) update(bat *battery.Battery, err error) {
	if err != nil {
		d.percentage.SetText("Error")
		d.status.SetText(err.Error())
		d.widget.SetBattery(nil)
		return
	}

	if bat == nil {
		d.percentage.SetText("No battery")
		d.status.SetText("")
		d.widget.SetBattery(nil)
		return
	}

	d.percentage.SetText(fmt.Sprintf("%d%%", bat.Percentage))
	d.widget.SetBattery(bat)

	// Smooth the time-left estimate.
	if bat.TimeLeft > 0 {
		smoothed := d.ema.update(bat.TimeLeft)
		d.status.SetText(fmt.Sprintf("%s · %s", bat.Status, formatDuration(smoothed)))
	} else {
		d.status.SetText(string(bat.Status))
	}
}
