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

	// Explicit subcommands. Anything else (no args) is the GUI.
	args := flag.Args()
	if len(args) > 0 {
		switch args[0] {
		case "daemon":
			runDaemon()
		case "install":
			runInstall()
		case "uninstall":
			runUninstall()
		default:
			fmt.Fprintf(os.Stderr, "usage: sway-power [--debug] [daemon|install|uninstall]\n")
			os.Exit(2)
		}
		return
	}

	// GUI mode. Installation of the lid service is NOT done here: the
	// GUI shows an explicit "Install the lid service" action in the Lid
	// Settings section instead.
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

func runDaemon() {
	d, err := daemon.New(debug)
	if err != nil {
		log.Fatalf("daemon: %v", err)
	}
	// Shutdown is owned here, not in d.Run(): it happens exactly once.
	defer d.Shutdown()
	d.Run()
}

// runInstall explicitly installs (and starts) the sway-power user service.
func runInstall() {
	binaryPath, err := os.Executable()
	if err != nil {
		log.Fatalf("install: %v", err)
	}
	binaryPath, err = filepath.Abs(binaryPath)
	if err != nil {
		log.Fatalf("install: %v", err)
	}
	if err := bootstrap.Bootstrap(binaryPath); err != nil {
		log.Fatalf("install: %v", err)
	}
	fmt.Println("sway-power: installed", bootstrap.UnitName())
}

// runUninstall explicitly removes the sway-power user service.
func runUninstall() {
	if err := bootstrap.Uninstall(); err != nil {
		log.Fatalf("uninstall: %v", err)
	}
	fmt.Println("sway-power: uninstalled", bootstrap.UnitName())
}
