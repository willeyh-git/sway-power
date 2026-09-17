package ui

import (
	"fmt"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/willeyh-git/sway-power/internal/bootstrap"
	"github.com/willeyh-git/sway-power/internal/lid/action"
	"github.com/willeyh-git/sway-power/internal/logger"
	"github.com/willeyh-git/sway-power/internal/preferences"
)

// lidCloseButtons handles the Lid Settings section: the lid close action
// buttons (available only when the lid service is installed) and the
// install/uninstall actions for the service itself.
type lidCloseButtons struct {
	actions []struct {
		id    string
		label string
	}
	btns          []*lidCloseBtn
	bar           *fyne.Container
	installLink   *widget.Hyperlink
	uninstallLink *widget.Hyperlink
	section       *fyne.Container
	status        setTextable
	label         fyne.CanvasObject
	current       string
	installed     bool
	inFlight      bool
	log           *logger.Logger
}

type lidCloseBtn struct {
	widget *toggleButtonWidget
	label  *canvas.Text
}

func newLidCloseButtons(debug bool, pal Palette, status setTextable) *lidCloseButtons {
	actions := []struct {
		id    string
		label string
	}{
		{"lock", "Lock"},
		{"sleep", "Sleep"},
		{"nothing", "Nothing"},
	}

	lg := logger.New(debug, "[lid] ")

	// Load current preference.
	prefs, err := preferences.Load()
	if err != nil {
		lg.Printf("failed to load preferences: %v", err)
		prefs = preferences.Default()
	}
	current := prefs.LidClose

	mgr := &lidCloseButtons{
		actions:   actions,
		status:    status,
		label:     canvas.NewText("Lid Settings", pal.Category),
		current:   current,
		installed: bootstrap.IsInstalled(),
		log:       lg,
	}
	mgr.label.(*canvas.Text).TextSize = theme.Size(SmallSize)
	mgr.label.(*canvas.Text).TextStyle = fyne.TextStyle{Bold: true}
	mgr.label.(*canvas.Text).Alignment = fyne.TextAlignLeading
	mgr.label.(*canvas.Text).Color = pal.Category

	var btnObjects []fyne.CanvasObject
	for _, a := range actions {
		w := newToggleButtonWidget(a.label, pal)
		w.SetActive(a.id == current)
		btn := &lidCloseBtn{
			widget: w,
			label:  w.label,
		}
		act := a.id
		w.OnTap = func() {
			mgr.setAction(act)
		}
		mgr.btns = append(mgr.btns, btn)
		btnObjects = append(btnObjects, w)
	}
	mgr.bar = container.New(&btnBar{}, btnObjects...)

	mgr.installLink = newServiceLink("Install the lid service", mgr.install)
	mgr.uninstallLink = newServiceLink("Uninstall the lid service", mgr.uninstall)
	mgr.section = container.NewVBox()
	mgr.rebuildSection()

	// v1-min status line: daemon service state at startup. Transient
	// messages (lid button taps, power-profiles-daemon problems)
	// overwrite it until the next one.
	if mgr.installed {
		mgr.status.SetText(lidHandlerStatus())
	} else {
		mgr.status.SetText("Lid service: not installed")
	}

	return mgr
}

// newServiceLink makes a hyperlink-styled action (plain text in the
// accent color, web-hyperlink look) for a lid service operation.
func newServiceLink(text string, onTap func()) *widget.Hyperlink {
	link := widget.NewHyperlink(text, nil)
	link.SizeName = SmallSize
	link.OnTapped = onTap
	return link
}

// rebuildSection fills the Lid Settings section with either the install
// link (service not installed) or the action buttons plus the uninstall
// link (service installed).
func (mgr *lidCloseButtons) rebuildSection() {
	if mgr.installed {
		spacer := canvas.NewRectangle(color.Transparent)
		spacer.Resize(fyne.NewSize(0, 2))
		mgr.section.Objects = []fyne.CanvasObject{mgr.bar, spacer, mgr.uninstallLink}
	} else {
		mgr.section.Objects = []fyne.CanvasObject{mgr.installLink}
	}
	mgr.section.Refresh()
}

// install runs the explicit service install (same code path as
// `sway-power install`) and then reveals the lid options. Taps are
// ignored while an install/uninstall is in flight so two operations can
// never interleave.
func (mgr *lidCloseButtons) install() {
	if mgr.inFlight {
		return
	}
	mgr.inFlight = true
	mgr.status.SetText("Installing the lid service…")
	go func() {
		execPath, err := os.Executable()
		if err == nil {
			execPath, err = filepath.Abs(execPath)
		}
		var installErr error
		if err == nil {
			installErr = bootstrap.Bootstrap(execPath)
		} else {
			installErr = err
		}
		fyne.Do(func() {
			mgr.inFlight = false
			if installErr != nil {
				mgr.log.Printf("install: %v", installErr)
				mgr.status.SetText(fmt.Sprintf("Install failed: %v", installErr))
				return
			}
			mgr.installed = true
			mgr.rebuildSection()
			mgr.status.SetText("Lid service installed")
		})
	}()
}

// uninstall removes the service (same code path as `sway-power
// uninstall`) and hides the lid options again.
func (mgr *lidCloseButtons) uninstall() {
	if mgr.inFlight {
		return
	}
	mgr.inFlight = true
	mgr.status.SetText("Uninstalling the lid service…")
	go func() {
		err := bootstrap.Uninstall()
		fyne.Do(func() {
			mgr.inFlight = false
			if err != nil {
				mgr.log.Printf("uninstall: %v", err)
				mgr.status.SetText(fmt.Sprintf("Uninstall failed: %v", err))
				return
			}
			mgr.installed = false
			mgr.rebuildSection()
			mgr.status.SetText("Lid service uninstalled")
		})
	}()
}

// lidSection returns the whole Lid Settings section (install link or
// action buttons + uninstall link).
func (mgr *lidCloseButtons) lidSection() *fyne.Container {
	return mgr.section
}

// lidHandlerStatus returns the v1-min status line: whether the
// sway-power daemon unit is active, per `systemctl --user is-active`.
// Note: is-active proves the process is running, not that the inhibit
// lock was acquired.
func lidHandlerStatus() string {
	out, err := exec.Command("systemctl", "--user", "is-active", "sway-power.service").Output()
	if err != nil {
		return "lid handler: inactive"
	}
	if strings.TrimSpace(string(out)) == "active" {
		return "lid handler: active"
	}
	return "lid handler: inactive"
}

func (mgr *lidCloseButtons) setAction(act string) {
	mgr.current = act

	// Update buttons.
	for i, a := range mgr.actions {
		mgr.btns[i].widget.SetActive(a.id == act)
	}

	// Save preference atomically.
	prefs, err := preferences.Load()
	if err != nil {
		mgr.log.Printf("failed to load preferences: %v", err)
		return
	}
	prefs.LidClose = act
	if err := preferences.Save(prefs); err != nil {
		mgr.log.Printf("failed to save preferences: %v", err)
	}

	// Update status.
	mgr.status.SetText("Lid action: " + act)
}

func (mgr *lidCloseButtons) labelText() fyne.CanvasObject {
	return mgr.label
}

// ValidateAction checks that the action is valid.
func ValidateAction(a string) error {
	return action.Action(a).Validate()
}
