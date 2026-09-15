package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"fyne.io/fyne/v2/app"

	"github.com/willeyh-git/sway-power/internal/config"
	"github.com/willeyh-git/sway-power/internal/ui"
)

var (
	debug bool
)

func main() {
	flag.BoolVar(&debug, "debug", false, "print D-Bus activity to stderr")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	a := app.NewWithID("com.willeyh.sway-power")

	if err := ui.Show(a, cfg, debug); err != nil {
		fmt.Fprintf(os.Stderr, "sway-power: %v\n", err)
		os.Exit(1)
	}
}
