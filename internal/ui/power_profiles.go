package ui

import (
	"fmt"
	"image/color"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/willeyh-git/sway-power/internal/config"
	"github.com/willeyh-git/sway-power/internal/power"
)

// active/inactive colors for profile buttons.
var (
	activeColor   = color.NRGBA{0x50, 0xc8, 0x78, 0xff} // green
	inactiveColor = color.NRGBA{0x88, 0x88, 0x88, 0xff} // gray
)

// powerProfileManager handles power profile buttons and D-Bus connection.
type powerProfileManager struct {
	profiles  []struct{ id, label string }
	btns      []*profileBtn
	bar       *fyne.Container
	status    setTextable
	label     fyne.CanvasObject
	pm        *power.Manager
	stopWatch func()
}

type profileBtn struct {
	widget *profileButtonWidget
	label  *canvas.Text
}

func newPowerProfileManager(debug bool, cfg config.Config, status setTextable) *powerProfileManager {
	profiles := []struct {
		id    string
		label string
	}{
		{power.ProfilePowerSaver, "Eco"},
		{power.ProfileBalanced, "Normal"},
		{power.ProfilePerformance, "Performance"},
	}

	ppm := &powerProfileManager{
		profiles: profiles,
		status:   status,
		label:    canvas.NewText("Power Profile", parseHexColor(cfg.UI.Category)),
	}
	ppm.label.(*canvas.Text).TextSize = theme.Size(SmallSize)
	ppm.label.(*canvas.Text).TextStyle = fyne.TextStyle{Bold: true}
	ppm.label.(*canvas.Text).Alignment = fyne.TextAlignLeading
	ppm.label.(*canvas.Text).Color = parseHexColor(cfg.UI.Category)

	var btnObjects []fyne.CanvasObject
	for i, p := range profiles {
		w := newProfileButtonWidget(p.label, false)
		btn := &profileBtn{
			widget: w,
			label:  w.label,
		}
		idx, prof := i, p.id
		w.OnTap = func() {
			ppm.setProfile(idx, prof)
		}
		ppm.btns = append(ppm.btns, btn)
		btnObjects = append(btnObjects, w)
	}

	ppm.bar = container.New(&btnBar{}, btnObjects...)

	// Connect to daemon asynchronously.
	go ppm.connect(debug)

	return ppm
}

func (mgr *powerProfileManager) connect(debug bool) {
	p, err := power.New(debug)
	if err != nil {
		fyne.Do(func() {
			mgr.status.SetText("power-profiles-daemon not running — install it to switch profiles")
		})
		return
	}
	mgr.pm = p

	// Sync buttons to current profile.
	active, _ := p.ActiveProfile()
	fyne.Do(func() {
		mgr.updateButtons(active)
	})

	// Watch for external profile changes.
	stopWatch := p.WatchActiveProfile(debug, func(profile string) {
		fyne.Do(func() {
			mgr.updateButtons(profile)
		})
	})
	mgr.stopWatch = stopWatch
}

func (mgr *powerProfileManager) updateButtons(active string) {
	for i, p := range mgr.profiles {
		if p.id == active {
			mgr.btns[i].widget.background.FillColor = activeColor
		} else {
			mgr.btns[i].widget.background.FillColor = inactiveColor
		}
	}
	for _, b := range mgr.btns {
		b.widget.Refresh()
	}
}

func (mgr *powerProfileManager) setProfile(idx int, prof string) {
	if mgr.pm == nil {
		return
	}
	if err := mgr.pm.SetActiveProfile(prof); err != nil {
		fyne.Do(func() {
			mgr.status.SetText("Failed: " + err.Error())
		})
		return
	}
	fyne.Do(func() {
		mgr.updateButtons(prof)
	})
}

func (mgr *powerProfileManager) destroy() {
	if mgr.stopWatch != nil {
		mgr.stopWatch()
	}
	if mgr.pm != nil {
		mgr.pm.Close()
	}
}

func (mgr *powerProfileManager) buttonBar() *fyne.Container {
	return mgr.bar
}

func (mgr *powerProfileManager) labelText() fyne.CanvasObject {
	return mgr.label
}

// profileButtonWidget is a clickable widget with a solid rectangular background.
type profileButtonWidget struct {
	widget.BaseWidget
	label        *canvas.Text
	background   *canvas.Rectangle
	OnTap        func()
}

func newProfileButtonWidget(label string, active bool) *profileButtonWidget {
	background := canvas.NewRectangle(inactiveColor)
	labelWidget := canvas.NewText(label, theme.Color(theme.ColorNameForeground))
	labelWidget.TextSize = theme.Size(SmallSize)
	labelWidget.TextStyle = fyne.TextStyle{Bold: true}

	w := &profileButtonWidget{
		label:        labelWidget,
		background:   background,
	}
	w.ExtendBaseWidget(w)

	if active {
		background.FillColor = activeColor
	}

	return w
}

func (w *profileButtonWidget) CreateRenderer() fyne.WidgetRenderer {
	return &profileButtonRenderer{
		widget: w,
		objects: []fyne.CanvasObject{
			w.background,
			w.label,
		},
	}
}

func (w *profileButtonWidget) Tapped(_ *fyne.PointEvent) {
	fmt.Fprintf(os.Stderr, "[ui] button tapped\n")
	if w.OnTap != nil {
		w.OnTap()
	}
}

type profileButtonRenderer struct {
	widget  *profileButtonWidget
	objects []fyne.CanvasObject
}

func (r *profileButtonRenderer) Layout(size fyne.Size) {
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

func (r *profileButtonRenderer) MinSize() fyne.Size {
	labelMin := r.widget.label.MinSize()
	return fyne.NewSize(labelMin.Width+20, labelMin.Height+10)
}

func (r *profileButtonRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *profileButtonRenderer) Refresh() {
	r.Layout(r.widget.Size())
	r.widget.background.Refresh()
	r.widget.label.Refresh()
}

func (r *profileButtonRenderer) Destroy() {}

func (r *profileButtonRenderer) Hovered()     {}
func (r *profileButtonRenderer) Unhovered()   {}
func (r *profileButtonRenderer) FocusGained() {}
func (r *profileButtonRenderer) FocusLost()   {}
