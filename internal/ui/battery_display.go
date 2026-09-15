package ui

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/willeyh-git/sway-power/internal/battery"
	"github.com/willeyh-git/sway-power/internal/config"
)

// emaSmooth applies exponential moving average smoothing to a time.Duration.
type emaSmooth struct {
	value time.Duration
	alpha float32
}

func newEMASmooth(alpha float32) *emaSmooth {
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
	n := float32(newVal)
	o := float32(e.value)
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

func (l *richTextLabel) SetText(text string) {
	l.rt.Segments = []widget.RichTextSegment{
		&widget.TextSegment{Text: text, Style: l.style},
	}
	l.rt.Refresh()
}

func (l *richTextLabel) Object() fyne.CanvasObject {
	return l.rt
}

// batteryDisplay builds and updates the battery section.
type batteryDisplay struct {
	icon       *canvas.Text
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

func newBatteryDisplay(cfg config.Config) (*fyne.Container, *batteryDisplay) {
	header := widget.RichTextStyle{SizeName: HeadingSize}
	headerBold := widget.RichTextStyle{
		SizeName:  HeadingSize,
		TextStyle: fyne.TextStyle{Bold: true},
	}
	small := widget.RichTextStyle{SizeName: SmallSize}
	smallBold := widget.RichTextStyle{
		SizeName:  SmallSize,
		TextStyle: fyne.TextStyle{Bold: true},
	}

	// Header: [icon] Battery [percentage%]
	iconSize := theme.Size(HeadingSize)
	icon := canvas.NewText("󰁹", parseHexColor(cfg.UI.Icon))
	icon.TextStyle = fyne.TextStyle{Monospace: true, Bold: true}
	icon.TextSize = iconSize
	percentage := newRichTextLabel("", headerBold)
	title := newRichTextLabel("Battery", header)

	// Grid: labels left, values right
	sizeLabel := newRichTextLabel("Battery size:", small)
	sizeValue := newRichTextLabel("", smallBold)
	timeLabel := newRichTextLabel("Time left:", small)
	timeValue := newRichTextLabel("", smallBold)
	cycleLabel := newRichTextLabel("Charge cycles:", small)
	cycleValue := newRichTextLabel("", smallBold)
	rateLabel := newRichTextLabel("Discharging:", small)
	rateValue := newRichTextLabel("", smallBold)

	// Header row: icon | Battery(grows) | percentage
	headerRow := container.New(&flexRow{growIdx: 1, gap: 4}, icon, title.Object(), percentage.Object())

	// Grid rows: label left, value right
	row1 := container.New(&labelValue{}, sizeLabel.Object(), sizeValue.Object())
	row2 := container.New(&labelValue{}, timeLabel.Object(), timeValue.Object())
	row3 := container.New(&labelValue{}, cycleLabel.Object(), cycleValue.Object())
	row4 := container.New(&labelValue{}, rateLabel.Object(), rateValue.Object())

	// 2x2 grid
	grid := container.New(&grid2x2{colGap: 20}, row1, row2, row3, row4)

	// Stack header and grid
	content := container.NewVBox(headerRow, grid)

	return content, &batteryDisplay{
		icon:       icon,
		percentage: percentage,
		sizeValue:  sizeValue,
		timeLabel:  timeLabel,
		timeValue:  timeValue,
		cycleValue: cycleValue,
		rateLabel:  rateLabel,
		rateValue:  rateValue,
		ema:        newEMASmooth(0.2),
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
	d.setIcon(bat.Percentage, bat.Status)

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

	if bat.Status == battery.StatusCharging {
		d.timeLabel.SetText("Time to full:")
	} else {
		d.timeLabel.SetText("Time left:")
	}

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
		d.rateLabel.SetText("Charging:")
	case battery.StatusDischarging:
		d.rateLabel.SetText("Discharging:")
	default:
		d.rateLabel.SetText("Power:")
	}
	if bat.RateW > 0 {
		d.rateValue.SetText(fmt.Sprintf("%.1f W", bat.RateW))
	} else {
		d.rateValue.SetText("")
	}
}

var iconLevels = []string{
	"󰁺", "󰁻", "󰁼", "󰁽", "󰁾", "󰁿", "󰂀", "󰂁", "󰂂", "󰁹",
}

func (d *batteryDisplay) setIcon(percentage int, status battery.Status) {
	switch status {
	case battery.StatusCharging:
		d.icon.Text = "󰂄"
	case battery.StatusFull:
		d.icon.Text = "󰚥"
	case battery.StatusNotCharging, battery.StatusUnknown:
		d.icon.Text = "󰚥"
	default:
		idx := percentage / 10
		if idx >= len(iconLevels) {
			idx = len(iconLevels) - 1
		}
		if idx < 0 {
			idx = 0
		}
		d.icon.Text = iconLevels[idx]
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
