package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"

	"github.com/willeyh-git/sway-power/internal/config"
	"github.com/willeyh-git/sway-power/internal/power"
)

// active/inactive colors for profile buttons.
var (
	activeColor   = color.NRGBA{0xd0, 0xd0, 0xd0, 0xff} // subtle gray
	inactiveColor = color.NRGBA{0x88, 0x88, 0x88, 0xff} // gray
)

// powerProfileManager handles power profile buttons and D-Bus connection.
type powerProfileManager struct {
	profiles     []struct{ id, label string }
	btns         []*profileBtn
	bar          *fyne.Container
	status       setTextable
	label        fyne.CanvasObject
	pm           *power.Manager
	stopWatch    func()
	bgColor      color.NRGBA
	activeColor  color.NRGBA
	accentColor  color.NRGBA
}

type profileBtn struct {
	widget *toggleButtonWidget
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
		profiles:    profiles,
		status:      status,
		label:       canvas.NewText("Power Profile", parseHexColor(cfg.UI.Category)),
		bgColor:     parseHexColor(cfg.UI.Background),
		activeColor: parseHexColor(cfg.UI.Active),
		accentColor: parseHexColor(cfg.UI.Accent),
	}
	ppm.label.(*canvas.Text).TextSize = theme.Size(SmallSize)
	ppm.label.(*canvas.Text).TextStyle = fyne.TextStyle{Bold: true}
	ppm.label.(*canvas.Text).Alignment = fyne.TextAlignLeading
	ppm.label.(*canvas.Text).Color = parseHexColor(cfg.UI.Category)

	var btnObjects []fyne.CanvasObject
	for i, p := range profiles {
		w := newToggleButtonWidget(p.label, false, parseHexColor(cfg.UI.Border), parseHexColor(cfg.UI.Value), parseHexColor(cfg.UI.Active), parseHexColor(cfg.UI.Background), parseHexColor(cfg.UI.Accent))
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
		mgr.updateButtons(active, mgr.bgColor, mgr.activeColor, mgr.accentColor)
	})

	// Watch for external profile changes.
	stopWatch := p.WatchActiveProfile(debug, func(profile string) {
		fyne.Do(func() {
			mgr.updateButtons(profile, mgr.bgColor, mgr.activeColor, mgr.accentColor)
		})
	})
	mgr.stopWatch = stopWatch
}

func (mgr *powerProfileManager) updateButtons(active string, bgColor color.NRGBA, activeColor color.NRGBA, accentColor color.NRGBA) {
	for i, p := range mgr.profiles {
		if p.id == active {
			mgr.btns[i].widget.background.FillColor = accentColor
			mgr.btns[i].widget.label.Color = color.White // White text on accent background
		} else {
			mgr.btns[i].widget.background.FillColor = bgColor
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
		mgr.updateButtons(prof, mgr.bgColor, mgr.activeColor, mgr.accentColor)
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
