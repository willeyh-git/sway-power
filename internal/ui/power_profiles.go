package ui

import (
	"fmt"
	"image/color"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

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
	pm        *power.Manager
	stopWatch func()
}

type profileBtn struct {
	widget *profileButtonWidget
	label  *widget.Label
	circle *canvas.Circle
}

func newPowerProfileManager(debug bool, status setTextable) *powerProfileManager {
	profiles := []struct {
		id    string
		label string
	}{
		{power.ProfilePowerSaver, "Eco"},
		{power.ProfileBalanced, "Normal"},
		{power.ProfilePerformance, "Performance"},
	}

	bar := container.NewHBox()

	ppm := &powerProfileManager{
		profiles: profiles,
		status:   status,
	}

	for i, p := range profiles {
		w := newProfileButtonWidget(p.label, false)
		btn := &profileBtn{
			widget: w,
			label:  w.label,
			circle: w.circle,
		}
		idx, prof := i, p.id
		w.OnTap = func() {
			ppm.setProfile(idx, prof)
		}
		ppm.btns = append(ppm.btns, btn)
		bar.Add(w)
	}

	ppm.bar = bar

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
			mgr.btns[i].circle.FillColor = activeColor
		} else {
			mgr.btns[i].circle.FillColor = inactiveColor
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

// profileButtonWidget is a clickable widget that shows a dot indicator and label.
type profileButtonWidget struct {
	widget.BaseWidget
	label  *widget.Label
	circle *canvas.Circle
	OnTap  func()
}

func newProfileButtonWidget(label string, active bool) *profileButtonWidget {
	circle := canvas.NewCircle(inactiveColor)
	circle.Resize(fyne.NewSize(12, 12))
	labelWidget := widget.NewLabel(label)
	labelWidget.Alignment = fyne.TextAlignCenter

	w := &profileButtonWidget{
		label:  labelWidget,
		circle: circle,
	}
	w.ExtendBaseWidget(w)

	if active {
		circle.FillColor = activeColor
	}

	return w
}

func (w *profileButtonWidget) CreateRenderer() fyne.WidgetRenderer {
	return &profileButtonRenderer{
		widget: w,
		objects: []fyne.CanvasObject{
			w.circle,
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

func (r *profileButtonRenderer) MinSize() fyne.Size {
	labelMin := r.widget.label.MinSize()
	return fyne.NewSize(12+4+labelMin.Width, labelMin.Height)
}

func (r *profileButtonRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *profileButtonRenderer) Refresh() {
	r.Layout(r.widget.Size())
	r.widget.circle.Refresh()
	r.widget.label.Refresh()
}

func (r *profileButtonRenderer) Destroy() {}

func (r *profileButtonRenderer) Hovered()     {}
func (r *profileButtonRenderer) Unhovered()   {}
func (r *profileButtonRenderer) FocusGained() {}
func (r *profileButtonRenderer) FocusLost()   {}
