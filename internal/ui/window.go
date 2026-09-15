package ui

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/willeyh-git/sway-power/internal/battery"
)

func Show(app fyne.App) {
	window := app.NewWindow("Sway Power")

	title := widget.NewLabel("Battery")
	title.Alignment = fyne.TextAlignCenter

	percentage := widget.NewLabel("")
	percentage.Alignment = fyne.TextAlignCenter

	status := widget.NewLabel("")
	status.Alignment = fyne.TextAlignCenter

	progress := widget.NewProgressBar()

	content := container.NewVBox(
		title,
		percentage,
		progress,
		status,
	)

	window.SetContent(
		container.NewCenter(content),
	)

	window.Resize(fyne.NewSize(400, 200))

	// Update the UI with battery information.
	updateUI := func(bat *battery.Battery, err error) {
		if err != nil {
			percentage.SetText("Error")
			status.SetText(err.Error())
			progress.SetValue(0)
			return
		}

		if bat == nil {
			percentage.SetText("No battery")
			status.SetText("")
			progress.SetValue(0)
			return
		}

		percentage.SetText(fmt.Sprintf("%d%%", bat.Percentage))
		status.SetText(string(bat.Status))
		progress.SetValue(float64(bat.Percentage) / 100)
	}

	// Read the initial battery state.
	bat, err := battery.Read()
	updateUI(bat, err)

	// Refresh the battery state periodically.
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			bat, err := battery.Read()

			// Fyne UI updates must happen on the Fyne call thread.
			fyne.Do(func() {
				updateUI(bat, err)
			})
		}
	}()

	window.ShowAndRun()
}
