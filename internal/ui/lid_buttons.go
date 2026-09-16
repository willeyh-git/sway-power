package ui

import (
	"os/exec"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"

	"github.com/willeyh-git/sway-power/internal/lid/action"
	"github.com/willeyh-git/sway-power/internal/logger"
	"github.com/willeyh-git/sway-power/internal/preferences"
)

// lidCloseButtons handles lid close action selection.
type lidCloseButtons struct {
	actions []struct {
		id    string
		label string
	}
	btns    []*lidCloseBtn
	bar     *fyne.Container
	status  setTextable
	label   fyne.CanvasObject
	current string
	log     *logger.Logger
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
		actions: actions,
		status:  status,
		label:   canvas.NewText("Lid Settings", pal.Category),
		current: current,
		log:     lg,
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

	// v1-min status line: daemon service state at startup. Transient
	// messages (lid button taps, power-profiles-daemon problems)
	// overwrite it until the next one.
	mgr.status.SetText(lidHandlerStatus())

	return mgr
}

// lidHandlerStatus returns the v1-min status line: whether the
// sway-power daemon unit is active, per `systemctl --user is-active`.
// Note: is-active proves the process is running, not that the inhibit
// lock was acquired (see "Daemon readiness" in docs/lid-daemon-plan.md).
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

func (mgr *lidCloseButtons) buttonBar() *fyne.Container {
	return mgr.bar
}

func (mgr *lidCloseButtons) labelText() fyne.CanvasObject {
	return mgr.label
}

// ValidateAction checks that the action is valid.
func ValidateAction(a string) error {
	return action.Action(a).Validate()
}
