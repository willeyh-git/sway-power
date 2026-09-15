package ui

import (
	"fmt"
	"image/color"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/willeyh-git/sway-power/internal/battery"
	"github.com/willeyh-git/sway-power/internal/config"
	"github.com/willeyh-git/sway-power/internal/power"
)

var (
	activeColor   = color.NRGBA{0x50, 0xc8, 0x78, 0xff} // green
	inactiveColor = color.NRGBA{0x88, 0x88, 0x88, 0xff} // gray
)

func Show(app fyne.App, cfg config.Config, debug bool) error {
	fmt.Fprintf(os.Stderr, "[ui] starting with debug=%v\n", debug)

	window := app.NewWindow("Sway Power")

	title := widget.NewLabel("Battery")
	title.Alignment = fyne.TextAlignCenter

	percentage := widget.NewLabel("")
	percentage.Alignment = fyne.TextAlignCenter

	status := widget.NewLabel("")
	status.Alignment = fyne.TextAlignCenter

	batteryWidget := NewBatteryWidget(cfg.Colors, nil)

	// Power profiles.
	profiles := []struct {
		id    string
		label string
	}{
		{power.ProfilePowerSaver, "Eco"},
		{power.ProfileBalanced, "Normal"},
		{power.ProfilePerformance, "Performance"},
	}

	// Create clickable profile buttons.
	type profileBtn struct {
		widget *profileButtonWidget
		label  *widget.Label
		circle *canvas.Circle
	}

	var btns []*profileBtn
	buttonBar := container.NewHBox()

	// Connect to power-profiles-daemon.
	var pm *power.Manager
	pm, err := power.New(debug)
	if err != nil {
		status.SetText("power-profiles-daemon not running — install it to switch profiles")
	} else {
		// Sync buttons to current profile.
		active, _ := pm.ActiveProfile()
		for i, p := range profiles {
			w := newProfileButtonWidget(p.label, p.id == active)
			btns = append(btns, &profileBtn{
				widget: w,
				label:  w.label,
				circle: w.circle,
			})
			btns[i].widget.OnTap = func(idx int, prof string) func() {
				return func() {
					if pm == nil {
						return
					}
					if err := pm.SetActiveProfile(prof); err != nil {
						status.SetText("Failed: " + err.Error())
						return
					}
					for j, b := range btns {
						if j == idx {
							b.circle.FillColor = activeColor
						} else {
							b.circle.FillColor = inactiveColor
						}
					}
				}
			}(i, p.id)
			buttonBar.Add(btns[i].widget)
		}
		// Watch for external profile changes.
		stopWatch := pm.WatchActiveProfile(debug, func(profile string) {
			fmt.Fprintf(os.Stderr, "[ui] signal: profile changed to %s\n", profile)
			fyne.Do(func() {
				for i, p := range profiles {
					if p.id == profile {
						btns[i].circle.FillColor = activeColor
					} else {
						btns[i].circle.FillColor = inactiveColor
					}
				}
				for _, b := range btns {
					b.widget.Refresh()
				}
			})
		})
		defer func() {
			stopWatch()
			pm.Close()
		}()
	}

	content := container.NewVBox(
		title,
		percentage,
		batteryWidget,
		status,
		widget.NewSeparator(),
		buttonBar,
	)

	window.SetContent(
		container.NewCenter(content),
	)

	window.Resize(fyne.NewSize(400, 240))

	updateUI := func(bat *battery.Battery, err error) {
		if err != nil {
			percentage.SetText("Error")
			status.SetText(err.Error())
			batteryWidget.SetBattery(nil)
			return
		}

		if bat == nil {
			percentage.SetText("No battery")
			status.SetText("")
			batteryWidget.SetBattery(nil)
			return
		}

		percentage.SetText(fmt.Sprintf("%d%%", bat.Percentage))
		status.SetText(string(bat.Status))
		batteryWidget.SetBattery(bat)
	}

	// Initial update after window is laid out.
	go func() {
		time.Sleep(50 * time.Millisecond)
		bat, err := battery.Read()
		fyne.Do(func() {
			updateUI(bat, err)
		})
	}()

	// Periodic updates.
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			bat, err := battery.Read()

			fyne.Do(func() {
				updateUI(bat, err)
			})
		}
	}()

	window.ShowAndRun()
	return nil
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
	labelMin := r.widget.label.MinSize()

	r.widget.circle.Move(fyne.NewPos(0, (size.Height-circleSize.Height)/2))
	r.widget.circle.Resize(circleSize)
	r.widget.label.Move(fyne.NewPos(circleSize.Width+4, (size.Height-labelMin.Height)/2))
	r.widget.label.Resize(labelMin)
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

func (r *profileButtonRenderer) Hovered()   {}
func (r *profileButtonRenderer) Unhovered()  {}
func (r *profileButtonRenderer) FocusGained() {}
func (r *profileButtonRenderer) FocusLost()  {}
