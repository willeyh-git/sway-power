package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"

	"github.com/willeyh-git/sway-power/internal/lid"
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
	btns      []*lidCloseBtn
	bar       *fyne.Container
	status    setTextable
	label     fyne.CanvasObject
	current   string
	stopWatch func()
	log       *logger.Logger
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

	// Start lid monitor.
	go mgr.startMonitor()

	return mgr
}

func (mgr *lidCloseButtons) startMonitor() {
	a := action.Action(mgr.current)
	if err := a.Validate(); err != nil {
		mgr.log.Printf("invalid action %q: %v", mgr.current, err)
		return
	}

	mgr.log.Printf("monitoring lid close, action=%s", mgr.current)

	stopWatch := lid.Monitor(func(state lid.State) {
		switch state {
		case lid.Closed:
			mgr.log.Printf("lid closed, executing %s", mgr.current)
			if err := a.Execute(); err != nil {
				mgr.log.Printf("failed to execute: %v", err)
			}
		case lid.Open:
			if err := a.OnOpen(); err != nil {
				mgr.log.Printf("failed to handle lid open: %v", err)
			}
		}
	}, mgr.log)
	mgr.stopWatch = stopWatch

	// Keep the goroutine alive.
	select {}
}

func (mgr *lidCloseButtons) setAction(act string) {
	mgr.current = act

	// Update buttons.
	for i, a := range mgr.actions {
		mgr.btns[i].widget.SetActive(a.id == act)
	}

	// Save preference.
	prefs, err := preferences.Load()
	if err != nil {
		mgr.log.Printf("failed to load preferences: %v", err)
		return
	}
	prefs.LidClose = act
	if err := preferences.Save(prefs); err != nil {
		mgr.log.Printf("failed to save preferences: %v", err)
	}

	// Restart the lid monitor with the new action.
	if mgr.stopWatch != nil {
		mgr.stopWatch()
	}
	go mgr.startMonitor()
}

func (mgr *lidCloseButtons) buttonBar() *fyne.Container {
	return mgr.bar
}

func (mgr *lidCloseButtons) labelText() fyne.CanvasObject {
	return mgr.label
}
