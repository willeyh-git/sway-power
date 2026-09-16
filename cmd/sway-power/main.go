// sway-power is a system tray power widget for sway.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2/app"

	"github.com/willeyh-git/sway-power/internal/bootstrap"
	"github.com/willeyh-git/sway-power/internal/config"
	"github.com/willeyh-git/sway-power/internal/daemon"
	"github.com/willeyh-git/sway-power/internal/ui"
)

var (
	debug bool
)

func main() {
	flag.BoolVar(&debug, "debug", false, "print D-Bus activity to stderr")
	flag.Parse()

	args := flag.Args()
	if len(args) > 0 && args[0] == "daemon" {
		runDaemon()
		return
	}

	// GUI mode: bootstrap daemon, then show UI.
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	// Bootstrap the systemd user service. Only the first launch does
	// real work; afterwards the unit is up to date and this is a no-op.
	binaryPath, err := os.Executable()
	if err == nil {
		// Resolve to absolute path.
		binaryPath, err = filepath.Abs(binaryPath)
		if err == nil {
			if err := bootstrap.Bootstrap(binaryPath); err != nil {
				// Non-fatal; the GUI can still run.
				fmt.Fprintf(os.Stderr, "sway-power: bootstrap warning: %v\n", err)
			}
		}
	}

	a := app.NewWithID("com.willeyh.sway-power")

	if err := ui.Show(a, cfg, debug); err != nil {
		fmt.Fprintf(os.Stderr, "sway-power: %v\n", err)
		os.Exit(1)
	}
}

func runDaemon() {
	d, err := daemon.New(debug)
	if err != nil {
		log.Fatalf("daemon: %v", err)
	}
	// Shutdown is owned here, not in d.Run(): it happens exactly once.
	defer d.Shutdown()
	d.Run()
}
