package main

import (
	"log"

	"fyne.io/fyne/v2/app"

	"github.com/willeyh-git/sway-power/internal/config"
	"github.com/willeyh-git/sway-power/internal/ui"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	a := app.NewWithID("com.willeyh.sway-power")

	ui.Show(a, cfg)
}
