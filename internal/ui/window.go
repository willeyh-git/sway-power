package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/willeyh-git/sway-power/internal/battery"
)

func Show(app fyne.App) {
	window := app.NewWindow("Sway Power")

	bat, err := battery.Read()

	if err != nil {
		window.SetContent(
			container.NewCenter(
				widget.NewLabel(fmt.Sprintf("Battery error: %v", err)),
			),
		)
	} else if bat == nil {
		window.SetContent(
			container.NewCenter(
				widget.NewLabel("No battery detected"),
			),
		)
	} else {
		title := widget.NewLabel("Battery")
		title.Alignment = fyne.TextAlignCenter

		percentage := widget.NewLabel(
			fmt.Sprintf("%d%%", bat.Percentage),
		)
		percentage.Alignment = fyne.TextAlignCenter

		status := widget.NewLabel(
			string(bat.Status),
		)
		status.Alignment = fyne.TextAlignCenter

		content := container.NewVBox(
			title,
			percentage,
			status,
		)

		window.SetContent(
			container.NewCenter(content),
		)
	}

	window.Resize(fyne.NewSize(400, 200))
	window.ShowAndRun()
}
