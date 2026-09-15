package ui

import (
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"os/exec"
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

	// Read scale factor from Sway/Wayland.
	if scale := getWaylandScale(); scale > 0 {
		os.Setenv("FYNE_SCALE", fmt.Sprintf("%.2f", scale))
	}

	window := app.NewWindow("Sway Power")
	window.SetFixedSize(true)
	window.Resize(fyne.NewSize(100, 140))

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

	// Create clickable profile buttons synchronously.
	type profileBtn struct {
		widget *profileButtonWidget
		label  *widget.Label
		circle *canvas.Circle
	}

	var btns []*profileBtn
	buttonBar := container.NewHBox()
	var pm *power.Manager

	for i, p := range profiles {
		w := newProfileButtonWidget(p.label, false)
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
					fyne.Do(func() {
						status.SetText("Failed: " + err.Error())
					})
					return
				}
				fyne.Do(func() {
					for j, b := range btns {
						if j == idx {
							b.circle.FillColor = activeColor
						} else {
							b.circle.FillColor = inactiveColor
						}
					}
				})
			}
		}(i, p.id)
		buttonBar.Add(btns[i].widget)
	}

	// Connect to power-profiles-daemon asynchronously.
	go func() {
		var p *power.Manager
		var err error
		p, err = power.New(debug)
		if err != nil {
			fyne.Do(func() {
				status.SetText("power-profiles-daemon not running — install it to switch profiles")
			})
			return
		}
		pm = p

		// Sync buttons to current profile.
		active, _ := pm.ActiveProfile()
		fyne.Do(func() {
			for i, p := range profiles {
				if p.id == active {
					btns[i].circle.FillColor = activeColor
				} else {
					btns[i].circle.FillColor = inactiveColor
				}
			}
			for _, b := range btns {
				b.widget.Refresh()
			}
		})

		// Watch for external profile changes.
		stopWatch := pm.WatchActiveProfile(debug, func(profile string) {
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

		// Block until the window closes.
		select {}
	}()

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

	// Updates: first one shortly after mapping (once the compositor has sent
	// the real scale), then every 5 seconds.
	go func() {
		time.Sleep(100 * time.Millisecond)

		bat, err := battery.Read()
		fyne.Do(func() {
			updateUI(bat, err)
			window.Content().Refresh() // one forced repaint at the real scale
		})

		ticker := time.NewTicker(5 * time.Second)
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

// getWaylandScale reads the display scale from Sway/Wayland.
// Returns 0 if the scale cannot be determined.
func getWaylandScale() float64 {
	// Try swaymsg first.
	cmd := exec.Command("swaymsg", "-t", "get_outputs")
	out, err := cmd.Output()
	if err != nil {
		return 0
	}

	type output struct {
		Scale float64 `json:"scale"`
	}

	var outputs []output
	if err := json.Unmarshal(out, &outputs); err != nil {
		return 0
	}

	// Return the first non-zero scale.
	for _, o := range outputs {
		if o.Scale > 0 {
			return o.Scale
		}
	}

	return 0
}
