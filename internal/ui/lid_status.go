package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"github.com/willeyh-git/sway-power/internal/preferences"
)

// lidStatusWidget is a widget that shows a context menu on tap.
type lidStatusWidget struct {
	widget.BaseWidget
	label   *widget.Label
	window  fyne.Window
	current string
	onTapped func()
}

func newLidStatusWidget(window fyne.Window, current string) *lidStatusWidget {
	w := &lidStatusWidget{
		window:  window,
		current: current,
	}

	// Create label with tap handler.
	w.label = widget.NewLabel("")
	w.label.Alignment = fyne.TextAlignCenter

	w.ExtendBaseWidget(w)

	return w
}

func (w *lidStatusWidget) CreateRenderer() fyne.WidgetRenderer {
	return &lidStatusRenderer{
		widget: w,
		objects: []fyne.CanvasObject{
			w.label,
		},
	}
}

func (w *lidStatusWidget) Tapped(_ *fyne.PointEvent) {
	if w.onTapped != nil {
		w.onTapped()
	}
}

func (w *lidStatusWidget) SetText(text string) {
	w.label.SetText(text)
}

func (w *lidStatusWidget) Refresh() {
	w.label.Refresh()
}

type lidStatusRenderer struct {
	widget  *lidStatusWidget
	objects []fyne.CanvasObject
}

func (r *lidStatusRenderer) Layout(size fyne.Size) {
	r.widget.label.Move(fyne.NewPos(0, 0))
	r.widget.label.Resize(size)
}

func (r *lidStatusRenderer) MinSize() fyne.Size {
	return r.widget.label.MinSize()
}

func (r *lidStatusRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *lidStatusRenderer) Refresh() {
	r.Layout(r.widget.Size())
	r.widget.label.Refresh()
}

func (r *lidStatusRenderer) Destroy() {}

// saveLidClose saves the lid close preference to disk.
func saveLidClose(win fyne.Window, value string) {
	_ = win // unused, but keeps the signature clear
	prefs, err := preferences.Load()
	if err != nil {
		return
	}
	prefs.LidClose = value
	_ = preferences.Save(prefs)
	// Note: the running lid monitor won't pick up the change until restart.
	// This is intentional — changing lid behavior mid-session could be dangerous.
}
