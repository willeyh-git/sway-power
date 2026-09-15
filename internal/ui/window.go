package ui

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/willeyh-git/sway-power/internal/battery"
	"github.com/willeyh-git/sway-power/internal/config"
)

func Show(app fyne.App, cfg config.Config) {
	window := app.NewWindow("Sway Power")

	title := widget.NewLabel("Battery")
	title.Alignment = fyne.TextAlignCenter

	percentage := widget.NewLabel("")
	percentage.Alignment = fyne.TextAlignCenter

	status := widget.NewLabel("")
	status.Alignment = fyne.TextAlignCenter

	batteryWidget := NewBatteryWidget(cfg.Colors, nil)

	content := container.NewVBox(
		title,
		percentage,
		batteryWidget,
		status,
	)

	window.SetContent(
		container.NewCenter(content),
	)

	window.Resize(fyne.NewSize(400, 220))

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

	// Defer initial update to run after window is shown and laid out.
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
}
