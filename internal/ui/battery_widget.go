package ui

import (
	"image/color"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"github.com/willeyh-git/sway-power/internal/battery"
	"github.com/willeyh-git/sway-power/internal/config"
)

type BatteryWidget struct {
	widget.BaseWidget

	Percentage int
	Status     battery.Status
	Colors     config.Colors
}

func NewBatteryWidget(cfg config.Colors, bat *battery.Battery) *BatteryWidget {
	w := &BatteryWidget{
		Colors: cfg,
	}

	if bat != nil {
		w.Percentage = bat.Percentage
		w.Status = bat.Status
	}

	w.ExtendBaseWidget(w)

	return w
}

func (w *BatteryWidget) SetBattery(bat *battery.Battery) {
	if bat == nil {
		w.Percentage = 0
		w.Status = battery.StatusUnknown
	} else {
		w.Percentage = bat.Percentage
		w.Status = bat.Status
	}

	w.Refresh()
}

func (w *BatteryWidget) CreateRenderer() fyne.WidgetRenderer {
	return newBatteryRenderer(w)
}

type batteryRenderer struct {
	widget *BatteryWidget

	track *canvas.Rectangle
	fill  *canvas.Rectangle

	objects []fyne.CanvasObject
}

func newBatteryRenderer(w *BatteryWidget) *batteryRenderer {
	r := &batteryRenderer{
		widget: w,
		track:  canvas.NewRectangle(parseColor(w.Colors.Track)),
		fill:   canvas.NewRectangle(parseColor(w.Colors.Normal)),
	}

	r.objects = []fyne.CanvasObject{
		r.track,
		r.fill,
	}

	return r
}

func (r *batteryRenderer) Layout(size fyne.Size) {
	height := float32(12)

	if size.Height < height {
		height = size.Height
	}

	y := (size.Height - height) / 2

	r.track.Move(fyne.NewPos(0, y))
	r.track.Resize(fyne.NewSize(size.Width, height))

	percentage := float32(r.widget.Percentage) / 100

	if percentage < 0 {
		percentage = 0
	}

	if percentage > 1 {
		percentage = 1
	}

	r.fill.Move(fyne.NewPos(0, y))
	r.fill.Resize(
		fyne.NewSize(
			size.Width*percentage,
			height,
		),
	)
}

func (r *batteryRenderer) MinSize() fyne.Size {
	return fyne.NewSize(300, 24)
}

func (r *batteryRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *batteryRenderer) Refresh() {
	r.updateColors()
	r.Layout(r.widget.Size())

	r.track.Refresh()
	r.fill.Refresh()
}

func (r *batteryRenderer) updateColors() {
	switch {
	case r.widget.Status == battery.StatusCharging:
		r.fill.FillColor = parseColor(r.widget.Colors.Charging)

	case r.widget.Percentage <= 15:
		r.fill.FillColor = parseColor(r.widget.Colors.Critical)

	case r.widget.Percentage <= 30:
		r.fill.FillColor = parseColor(r.widget.Colors.Warning)

	default:
		r.fill.FillColor = parseColor(r.widget.Colors.Normal)
	}

	r.track.FillColor = parseColor(r.widget.Colors.Track)
}

func (r *batteryRenderer) Destroy() {}

func parseColor(value string) color.NRGBA {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "#")

	if len(value) != 6 {
		return color.NRGBA{
			R: 128,
			G: 128,
			B: 128,
			A: 255,
		}
	}

	r, errR := strconv.ParseUint(value[0:2], 16, 8)
	g, errG := strconv.ParseUint(value[2:4], 16, 8)
	b, errB := strconv.ParseUint(value[4:6], 16, 8)

	if errR != nil || errG != nil || errB != nil {
		return color.NRGBA{
			R: 128,
			G: 128,
			B: 128,
			A: 255,
		}
	}

	return color.NRGBA{
		R: uint8(r),
		G: uint8(g),
		B: uint8(b),
		A: 255,
	}
}
