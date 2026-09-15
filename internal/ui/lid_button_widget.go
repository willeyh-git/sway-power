package ui

import (
	"fmt"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// lidCloseButtonWidget is a clickable widget with a solid rectangular background.
type lidCloseButtonWidget struct {
	widget.BaseWidget
	label      *canvas.Text
	background *canvas.Rectangle
	OnTap      func()
}

func newLidCloseButtonWidget(label string, active bool) *lidCloseButtonWidget {
	background := canvas.NewRectangle(inactiveColor)
	labelWidget := canvas.NewText(label, theme.Color(theme.ColorNameForeground))
	labelWidget.TextSize = theme.Size(SmallSize)
	labelWidget.TextStyle = fyne.TextStyle{Bold: true}

	w := &lidCloseButtonWidget{
		label:      labelWidget,
		background: background,
	}
	w.ExtendBaseWidget(w)

	if active {
		background.FillColor = activeColor
	}

	return w
}

func (w *lidCloseButtonWidget) CreateRenderer() fyne.WidgetRenderer {
	return &lidCloseButtonRenderer{
		widget: w,
		objects: []fyne.CanvasObject{
			w.background,
			w.label,
		},
	}
}

func (w *lidCloseButtonWidget) Tapped(_ *fyne.PointEvent) {
	fmt.Fprintf(os.Stderr, "[lid] button tapped\n")
	if w.OnTap != nil {
		w.OnTap()
	}
}

type lidCloseButtonRenderer struct {
	widget  *lidCloseButtonWidget
	objects []fyne.CanvasObject
}

func (r *lidCloseButtonRenderer) Layout(size fyne.Size) {
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

func (r *lidCloseButtonRenderer) MinSize() fyne.Size {
	labelMin := r.widget.label.MinSize()
	return fyne.NewSize(labelMin.Width+20, labelMin.Height+10)
}

func (r *lidCloseButtonRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *lidCloseButtonRenderer) Refresh() {
	r.Layout(r.widget.Size())
	r.widget.background.Refresh()
	r.widget.label.Refresh()
}

func (r *lidCloseButtonRenderer) Destroy() {}

func (r *lidCloseButtonRenderer) Hovered()     {}
func (r *lidCloseButtonRenderer) Unhovered()   {}
func (r *lidCloseButtonRenderer) FocusGained() {}
func (r *lidCloseButtonRenderer) FocusLost()   {}
