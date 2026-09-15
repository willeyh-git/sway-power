package ui

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
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

// richTextLabel is a single-line rich text label with a fixed style.
type richTextLabel struct {
	rt    *widget.RichText
	style widget.RichTextStyle
}

func newRichTextLabel(text string, style widget.RichTextStyle) *richTextLabel {
	return &richTextLabel{
		rt:    widget.NewRichText(&widget.TextSegment{Text: text, Style: style}),
		style: style,
	}
}

// SetText replaces the text, preserving the style.
func (l *richTextLabel) SetText(text string) {
	l.rt.Segments = []widget.RichTextSegment{
		&widget.TextSegment{Text: text, Style: l.style},
	}
	l.rt.Refresh()
}

// Object returns the underlying widget for layout.
func (l *richTextLabel) Object() fyne.CanvasObject {
	return l.rt
}

// batteryDisplay builds and updates the battery section: a header row with
// a battery icon, title and percentage in large text, followed by a
// two-by-two grid of details in small text where the values are bold.
type batteryDisplay struct {
	percentage *richTextLabel
	sizeValue  *richTextLabel
	timeLabel  *richTextLabel
	timeValue  *richTextLabel
	cycleValue *richTextLabel
	rateLabel  *richTextLabel
	rateValue  *richTextLabel
	ema        *emaSmooth
	lastStatus battery.Status
}

func newBatteryDisplay() (*fyne.Container, *batteryDisplay) {
	header := widget.RichTextStyle{SizeName: theme.SizeNameHeadingText}
	headerBold := widget.RichTextStyle{
		SizeName:  theme.SizeNameHeadingText,
		TextStyle: fyne.TextStyle{Bold: true},
	}
	small := widget.RichTextStyle{SizeName: theme.SizeNameCaptionText}
	smallBold := widget.RichTextStyle{
		SizeName:  theme.SizeNameCaptionText,
		TextStyle: fyne.TextStyle{Bold: true},
	}

	// Header: [ icon ] Battery        78%
	percentage := newRichTextLabel("", headerBold)
	title := newRichTextLabel("Battery", header)
	headerBar := container.NewHBox(newBatteryIcon(), title.Object(), layout.NewSpacer(), percentage.Object())

	// Grid: labels are regular, values are bold.
	sizeLabel := newRichTextLabel("Battery size:  ", small)
	sizeValue := newRichTextLabel("", smallBold)
	timeLabel := newRichTextLabel("Time left:  ", small)
	timeValue := newRichTextLabel("", smallBold)
	cycleLabel := newRichTextLabel("Charge cycles:  ", small)
	cycleValue := newRichTextLabel("", smallBold)
	rateLabel := newRichTextLabel("Discharging:  ", small)
	rateValue := newRichTextLabel("", smallBold)

	grid := container.NewGridWithColumns(2,
		container.NewHBox(sizeLabel.Object(), sizeValue.Object()),
		container.NewHBox(timeLabel.Object(), timeValue.Object()),
		container.NewHBox(cycleLabel.Object(), cycleValue.Object()),
		container.NewHBox(rateLabel.Object(), rateValue.Object()),
	)

	content := container.NewVBox(headerBar, grid)

	return content, &batteryDisplay{
		percentage: percentage,
		sizeValue:  sizeValue,
		timeLabel:  timeLabel,
		timeValue:  timeValue,
		cycleValue: cycleValue,
		rateLabel:  rateLabel,
		rateValue:  rateValue,
		ema:        newEMASmooth(0.2), // 20% weight on new reading
	}
}

func (d *batteryDisplay) update(bat *battery.Battery, err error) {
	if err != nil {
		d.percentage.SetText("Error")
		d.clearDetails()
		return
	}

	if bat == nil {
		d.percentage.SetText("No battery")
		d.clearDetails()
		return
	}

	d.percentage.SetText(fmt.Sprintf("%d%%", bat.Percentage))

	if bat.SizeWh > 0 {
		d.sizeValue.SetText(fmt.Sprintf("%d Wh", int(math.Round(bat.SizeWh))))
	} else {
		d.sizeValue.SetText("")
	}

	if bat.Cycles > 0 {
		d.cycleValue.SetText(strconv.Itoa(bat.Cycles))
	} else {
		d.cycleValue.SetText("")
	}

	// Time to full while charging, time left while discharging.
	if bat.Status == battery.StatusCharging {
		d.timeLabel.SetText("Time to full:  ")
	} else {
		d.timeLabel.SetText("Time left:  ")
	}

	// Smooth the time estimate, but never blend estimates from a
	// different status (time-to-full vs time-to-empty).
	if bat.Status != d.lastStatus {
		d.ema.reset()
		d.lastStatus = bat.Status
	}
	if bat.TimeLeft > 0 {
		d.timeValue.SetText(formatDuration(d.ema.update(bat.TimeLeft)))
	} else {
		d.timeValue.SetText("")
	}

	switch bat.Status {
	case battery.StatusCharging:
		d.rateLabel.SetText("Charging:  ")
	case battery.StatusDischarging:
		d.rateLabel.SetText("Discharging:  ")
	default:
		d.rateLabel.SetText("Power:  ")
	}
	if bat.RateW > 0 {
		d.rateValue.SetText(fmt.Sprintf("%.1f W", bat.RateW))
	} else {
		d.rateValue.SetText("")
	}
}

func (d *batteryDisplay) clearDetails() {
	d.sizeValue.SetText("")
	d.timeLabel.SetText("")
	d.timeValue.SetText("")
	d.cycleValue.SetText("")
	d.rateLabel.SetText("")
	d.rateValue.SetText("")
}
