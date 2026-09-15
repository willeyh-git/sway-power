package ui

import (
	"fmt"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

// lidCloseButtonWidget is a clickable widget for lid close actions.
type lidCloseButtonWidget struct {
	widget.BaseWidget
	label  *widget.Label
	circle *canvas.Circle
	OnTap  func()
}

func newLidCloseButtonWidget(label string, active bool) *lidCloseButtonWidget {
	circle := canvas.NewCircle(inactiveColor)
	circle.Resize(fyne.NewSize(12, 12))
	labelWidget := widget.NewLabel(label)
	labelWidget.Alignment = fyne.TextAlignCenter

	w := &lidCloseButtonWidget{
		label:  labelWidget,
		circle: circle,
	}
	w.ExtendBaseWidget(w)

	if active {
		circle.FillColor = activeColor
	}

	return w
}

func (w *lidCloseButtonWidget) CreateRenderer() fyne.WidgetRenderer {
	return &lidCloseButtonRenderer{
		widget: w,
		objects: []fyne.CanvasObject{
			w.circle,
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
	circleSize := fyne.NewSize(12, 12)

	// Position dot vertically centered
	r.widget.circle.Move(fyne.NewPos(0, (size.Height-circleSize.Height)/2))
	r.widget.circle.Resize(circleSize)

	// Give remaining width to label so text doesn't clip on scale changes
	labelX := circleSize.Width + 4
	labelWidth := size.Width - labelX
	if labelWidth < 0 {
		labelWidth = 0
	}

	labelMin := r.widget.label.MinSize()
	r.widget.label.Move(fyne.NewPos(labelX, (size.Height-labelMin.Height)/2))
	r.widget.label.Resize(fyne.NewSize(labelWidth, labelMin.Height))
}

func (r *lidCloseButtonRenderer) MinSize() fyne.Size {
	labelMin := r.widget.label.MinSize()
	return fyne.NewSize(12+4+labelMin.Width, labelMin.Height)
}

func (r *lidCloseButtonRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *lidCloseButtonRenderer) Refresh() {
	r.Layout(r.widget.Size())
	r.widget.circle.Refresh()
	r.widget.label.Refresh()
}

func (r *lidCloseButtonRenderer) Destroy() {}

func (r *lidCloseButtonRenderer) Hovered()     {}
func (r *lidCloseButtonRenderer) Unhovered()   {}
func (r *lidCloseButtonRenderer) FocusGained() {}
func (r *lidCloseButtonRenderer) FocusLost()   {}
