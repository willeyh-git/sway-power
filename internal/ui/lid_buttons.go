package ui

import (
	"fmt"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/willeyh-git/sway-power/internal/config"
	"github.com/willeyh-git/sway-power/internal/lid"
	"github.com/willeyh-git/sway-power/internal/lid/action"
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
}

type lidCloseBtn struct {
	widget *lidCloseButtonWidget
	label  *widget.Label
	circle *canvas.Circle
}

func newLidCloseButtons(debug bool, cfg config.Config, status setTextable) *lidCloseButtons {
	actions := []struct {
		id    string
		label string
	}{
		{"lock", "Lock"},
		{"sleep", "Sleep"},
		{"nothing", "Nothing"},
	}

	// Load current preference.
	prefs, err := preferences.Load()
	if err != nil {
		if debug {
			fmt.Fprintf(os.Stderr, "[lid] failed to load preferences: %v\n", err)
		}
		prefs = preferences.Default()
	}
	current := prefs.LidClose

	mgr := &lidCloseButtons{
		actions: actions,
		status:  status,
		label:   canvas.NewText("Lid Settings", parseHexColor(cfg.UI.Category)),
		current: current,
	}
	mgr.label.(*canvas.Text).TextSize = theme.Size(SmallSize)
	mgr.label.(*canvas.Text).TextStyle = fyne.TextStyle{Bold: true}
	mgr.label.(*canvas.Text).Alignment = fyne.TextAlignLeading
	mgr.label.(*canvas.Text).Color = parseHexColor(cfg.UI.Category)

	var btnObjects []fyne.CanvasObject
	for i, a := range actions {
		w := newLidCloseButtonWidget(a.label, a.id == current)
		btn := &lidCloseBtn{
			widget: w,
			label:  w.label,
			circle: w.circle,
		}
		idx, act := i, a.id
		w.OnTap = func() {
			mgr.setAction(idx, act)
		}
		mgr.btns = append(mgr.btns, btn)
		btnObjects = append(btnObjects, w)
	}
	mgr.bar = container.New(&btnBar{}, btnObjects...)

	// Start lid monitor.
	go mgr.startMonitor(debug)

	return mgr
}

func (mgr *lidCloseButtons) startMonitor(debug bool) {
	a := action.Action(mgr.current)
	if err := a.Validate(); err != nil {
		if debug {
			fmt.Fprintf(os.Stderr, "[lid] invalid action %q: %v\n", mgr.current, err)
		}
		return
	}

	if debug {
		fmt.Fprintf(os.Stderr, "[lid] monitoring lid close, action=%s\n", mgr.current)
	}

	stopWatch := lid.Monitor(func(state lid.State) {
		switch state {
		case lid.Closed:
			if debug {
				fmt.Fprintf(os.Stderr, "[lid] lid closed, executing %s\n", mgr.current)
			}
			if err := a.Execute(); err != nil {
				if debug {
					fmt.Fprintf(os.Stderr, "[lid] failed to execute %s: %v\n", mgr.current, err)
				}
			}
		case lid.Open:
			if err := a.OnOpen(); err != nil {
				if debug {
					fmt.Fprintf(os.Stderr, "[lid] failed to handle lid open: %v\n", err)
				}
			}
		}
	})
	mgr.stopWatch = stopWatch

	// Keep the goroutine alive.
	select {}
}

func (mgr *lidCloseButtons) setAction(idx int, act string) {
	mgr.current = act

	// Update buttons.
	for i, a := range mgr.actions {
		if a.id == act {
			mgr.btns[i].circle.FillColor = activeColor
		} else {
			mgr.btns[i].circle.FillColor = inactiveColor
		}
	}
	for _, b := range mgr.btns {
		b.widget.Refresh()
	}

	// Save preference.
	prefs, err := preferences.Load()
	if err != nil {
		return
	}
	prefs.LidClose = act
	_ = preferences.Save(prefs)

	// Restart the lid monitor with the new action.
	if mgr.stopWatch != nil {
		mgr.stopWatch()
	}
	go mgr.startMonitor(false)
}

func (mgr *lidCloseButtons) buttonBar() *fyne.Container {
	return mgr.bar
}

func (mgr *lidCloseButtons) labelText() fyne.CanvasObject {
	return mgr.label
}
