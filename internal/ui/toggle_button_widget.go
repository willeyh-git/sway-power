package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// toggleButtonWidget is a clickable widget with a solid rectangular background.
type toggleButtonWidget struct {
	widget.BaseWidget
	label      *canvas.Text
	background *canvas.Rectangle
	border     *canvas.Rectangle
	OnTap      func()
}

func newToggleButtonWidget(label string, active bool, borderColor, labelColor, activeColor color.NRGBA) *toggleButtonWidget {
	background := canvas.NewRectangle(theme.Color(theme.ColorNameBackground))
	border := canvas.NewRectangle(borderColor)
	border.StrokeWidth = 2
	border.StrokeColor = borderColor
	border.FillColor = color.Transparent
	labelWidget := canvas.NewText(label, labelColor)
	labelWidget.TextSize = theme.Size(SmallSize)
	labelWidget.TextStyle = fyne.TextStyle{Bold: true}

	w := &toggleButtonWidget{
		label:      labelWidget,
		background: background,
		border:     border,
	}
	w.ExtendBaseWidget(w)

	if active {
		background.FillColor = activeColor
	}

	return w
}

func (w *toggleButtonWidget) CreateRenderer() fyne.WidgetRenderer {
	return &toggleButtonRenderer{
		widget: w,
		objects: []fyne.CanvasObject{
			w.border,
			w.background,
			w.label,
		},
	}
}

func (w *toggleButtonWidget) Tapped(_ *fyne.PointEvent) {
	if w.OnTap != nil {
		w.OnTap()
	}
}

type toggleButtonRenderer struct {
	widget  *toggleButtonWidget
	objects []fyne.CanvasObject
}

func (r *toggleButtonRenderer) Layout(size fyne.Size) {
	// Resize border to fill the entire widget
	r.widget.border.Resize(size)
	r.widget.border.Move(fyne.NewPos(0, 0))

	// Resize background to fill the entire widget
	r.widget.background.Resize(size)
	r.widget.background.Move(fyne.NewPos(0, 0))

	// Center label in the background
	labelMin := r.widget.label.MinSize()
	labelX := (size.Width - labelMin.Width) / 2
	labelY := (size.Height - labelMin.Height) / 2
	r.widget.label.Move(fyne.NewPos(labelX, labelY))
	r.widget.label.Resize(labelMin)
}

func (r *toggleButtonRenderer) MinSize() fyne.Size {
	labelMin := r.widget.label.MinSize()
	return fyne.NewSize(labelMin.Width+20, labelMin.Height+10)
}

func (r *toggleButtonRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *toggleButtonRenderer) Refresh() {
	r.Layout(r.widget.Size())
	r.widget.border.Refresh()
	r.widget.background.Refresh()
	r.widget.label.Refresh()
}

func (r *toggleButtonRenderer) Destroy() {}

func (r *toggleButtonRenderer) Hovered()     {}
func (r *toggleButtonRenderer) Unhovered()   {}
func (r *toggleButtonRenderer) FocusGained() {}
func (r *toggleButtonRenderer) FocusLost()   {}
