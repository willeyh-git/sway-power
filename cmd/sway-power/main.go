package main

import (
	"fyne.io/fyne/v2/app"

	"github.com/willeyh-git/sway-power/internal/ui"
)

func main() {
	a := app.NewWithID("com.willeyh.sway-power")

	ui.Show(a)
}
