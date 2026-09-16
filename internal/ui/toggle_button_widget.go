package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// toggleButtonWidget is a clickable rectangular button. All colors come
// from the resolved Palette:
//   - selected:  ButtonActive background, OnActive text, ButtonBorder border
//   - hover/focus: ButtonHover background, OnHover text
//   - default:   Button background, ButtonLabel text, no border
type toggleButtonWidget struct {
	widget.BaseWidget
	pal        Palette
	label      *canvas.Text
	background *canvas.Rectangle
	border     *canvas.Rectangle
	active     bool
	hovered    bool
	focused    bool
	OnTap      func()
}

func newToggleButtonWidget(label string, pal Palette) *toggleButtonWidget {
	background := canvas.NewRectangle(pal.Button)
	border := canvas.NewRectangle(pal.ButtonBorder)
	labelWidget := canvas.NewText(label, pal.ButtonLabel)
	labelWidget.TextSize = theme.Size(SmallSize)

	w := &toggleButtonWidget{
		pal:        pal,
		label:      labelWidget,
		background: background,
		border:     border,
	}
	w.ExtendBaseWidget(w)
	w.restyle()

	return w
}

// SetActive marks the button as the selected one and restyles it.
func (w *toggleButtonWidget) SetActive(active bool) {
	w.active = active
	w.restyle()
	w.Refresh()
}

func (w *toggleButtonWidget) restyle() {
	var borderColor color.Color
	switch {
	case w.active:
		w.background.FillColor = w.pal.ButtonActive
		w.label.Color = w.pal.OnActive
		borderColor = w.pal.ButtonBorder
	case w.hovered || w.focused:
		w.background.FillColor = w.pal.ButtonHover
		w.label.Color = w.pal.OnHover
	default:
		w.background.FillColor = w.pal.Button
		w.label.Color = w.pal.ButtonLabel
	}
	w.border.FillColor = borderColor
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
	// Resize border to fill the entire widget (slightly larger to account for stroke)
	borderSize := fyne.NewSize(size.Width+2, size.Height+2)
	r.widget.border.Resize(borderSize)
	r.widget.border.Move(fyne.NewPos(-1, -1))

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

func (r *toggleButtonRenderer) Hovered() {
	r.widget.hovered = true
	r.widget.restyle()
	r.Refresh()
}

func (r *toggleButtonRenderer) Unhovered() {
	r.widget.hovered = false
	r.widget.restyle()
	r.Refresh()
}

func (r *toggleButtonRenderer) FocusGained() {
	r.widget.focused = true
	r.widget.restyle()
	r.Refresh()
}

func (r *toggleButtonRenderer) FocusLost() {
	r.widget.focused = false
	r.widget.restyle()
	r.Refresh()
}
