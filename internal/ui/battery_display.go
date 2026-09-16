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
)

// emaSmooth applies exponential moving average smoothing to a time.Duration.
type emaSmooth struct {
	value time.Duration
	alpha float32
}

func newEMASmooth(alpha float32) *emaSmooth {
	return &emaSmooth{
		alpha: alpha,
	}
}

func (e *emaSmooth) reset() {
	e.value = 0
}

func (e *emaSmooth) update(newVal time.Duration) time.Duration {
	if e.value == 0 {
		e.value = newVal
		return newVal
	}
	n := float64(newVal)
	o := float64(e.value)
	e.value = time.Duration(e.alpha*float32(n) + (1-e.alpha)*float32(o))
	return e.value
}

// setTextable is an interface for widgets that can have their text set.
type setTextable interface {
	SetText(string)
}

// richTextLabel is a single-line rich text label with a fixed style.
// The text color comes from the style's ColorName via the app theme, so
// every label stays themeable by the YAML config.
type richTextLabel struct {
	rt    *widget.RichText
	style widget.RichTextStyle
}

func newRichTextLabel(text string, style widget.RichTextStyle) *richTextLabel {
	return &richTextLabel{
		style: style,
		rt:    widget.NewRichText(&widget.TextSegment{Text: text, Style: style}),
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

// batteryTrack is a progress track: a shaded background with an accent
// fill showing the battery percentage.
type batteryTrack struct {
	widget.BaseWidget
	pal  Palette
	bg   *canvas.Rectangle
	fill *canvas.Rectangle
	pct  int
}

const trackHeight = 8

func newBatteryTrack(pal Palette) *batteryTrack {
	bg := canvas.NewRectangle(pal.Track)
	bg.CornerRadius = trackHeight / 2
	fill := canvas.NewRectangle(pal.Accent)
	fill.CornerRadius = trackHeight / 2

	t := &batteryTrack{
		pal:  pal,
		bg:   bg,
		fill: fill,
	}
	t.ExtendBaseWidget(t)
	return t
}

// SetPercentage sets the fill level (0-100) and refreshes.
func (t *batteryTrack) SetPercentage(pct int) {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	t.pct = pct
	t.Refresh()
}

func (t *batteryTrack) CreateRenderer() fyne.WidgetRenderer {
	return &batteryTrackRenderer{
		track:   t,
		objects: []fyne.CanvasObject{t.bg, t.fill},
	}
}

type batteryTrackRenderer struct {
	track   *batteryTrack
	objects []fyne.CanvasObject
}

func (r *batteryTrackRenderer) Layout(size fyne.Size) {
	r.track.bg.Resize(size)
	r.track.bg.Move(fyne.NewPos(0, 0))

	fillWidth := size.Width * float32(r.track.pct) / 100
	if fillWidth > size.Width {
		fillWidth = size.Width
	}
	r.track.fill.Resize(fyne.NewSize(fillWidth, size.Height))
	r.track.fill.Move(fyne.NewPos(0, 0))
}

func (r *batteryTrackRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, trackHeight)
}

func (r *batteryTrackRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *batteryTrackRenderer) Refresh() {
	r.Layout(r.track.Size())
	r.track.bg.Refresh()
	r.track.fill.Refresh()
}

func (r *batteryTrackRenderer) Destroy() {}

// batteryDisplay builds and updates the battery section.
type batteryDisplay struct {
	icon       *canvas.Text
	percentage *richTextLabel
	track      *batteryTrack
	sizeValue  *richTextLabel
	timeLabel  *richTextLabel
	timeValue  *richTextLabel
	cycleValue *richTextLabel
	rateLabel  *richTextLabel
	rateValue  *richTextLabel
	ema        *emaSmooth
	lastStatus battery.Status
}

func newBatteryDisplay(pal Palette) (*fyne.Container, *batteryDisplay) {
	header := widget.RichTextStyle{SizeName: HeadingSize, ColorName: themeNameTitle}
	headerBold := widget.RichTextStyle{
		SizeName:  HeadingSize,
		ColorName: themeNameValue,
		TextStyle: fyne.TextStyle{Bold: true},
	}
	small := widget.RichTextStyle{SizeName: SmallSize, ColorName: themeNameLabel}
	smallBold := widget.RichTextStyle{
		SizeName:  SmallSize,
		ColorName: themeNameValue,
		TextStyle: fyne.TextStyle{Bold: true},
	}

	// Header: [icon] Battery [percentage%]
	iconSize := theme.Size(HeadingSize)
	icon := canvas.NewText("󰁹", pal.Icon)
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

	track := newBatteryTrack(pal)

	// Header row: icon | Battery(grows) | percentage
	headerRow := container.New(&flexRow{growIdx: 1, gap: 4}, icon, title.Object(), percentage.Object())

	// Grid rows: label left, value right
	row1 := container.New(&labelValue{}, sizeLabel.Object(), sizeValue.Object())
	row2 := container.New(&labelValue{}, timeLabel.Object(), timeValue.Object())
	row3 := container.New(&labelValue{}, cycleLabel.Object(), cycleValue.Object())
	row4 := container.New(&labelValue{}, rateLabel.Object(), rateValue.Object())

	// 2x2 grid
	grid := container.New(&grid2x2{colGap: 20}, row1, row2, row3, row4)

	// Stack header, track and grid
	content := container.NewVBox(headerRow, track, grid)

	return content, &batteryDisplay{
		icon:       icon,
		percentage: percentage,
		track:      track,
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
		d.track.SetPercentage(0)
		d.clearDetails()
		return
	}
	if bat == nil {
		d.percentage.SetText("No battery")
		d.track.SetPercentage(0)
		d.clearDetails()
		return
	}

	d.percentage.SetText(fmt.Sprintf("%d%%", bat.Percentage))
	d.track.SetPercentage(bat.Percentage)
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
	"󰁺 ", "󰁻 ", "󰁼 ", "󰁽 ", "󰁾 ", "󰁿 ", "󰂀 ", "󰂁 ", "󰂂 ",
}

var chargingIconLevels = []string{
	"󰢜 ", "󰂆 ", "󰂇 ", "󰂈 ", "󰢝 ", "󰢞 ", "󰂊 ", "󰂋 ", "󰂅 ",
}

func (d *batteryDisplay) setIcon(percentage int, status battery.Status) {
	switch status {
	case battery.StatusCharging:
		idx := percentage / 10
		if idx >= len(chargingIconLevels) {
			idx = len(chargingIconLevels) - 1
		}
		if idx < 0 {
			idx = 0
		}
		d.icon.Text = chargingIconLevels[idx]
	case battery.StatusFull:
		d.icon.Text = "󰂄 "
	case battery.StatusNotCharging, battery.StatusUnknown:
		d.icon.Text = "󰁹 "
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
