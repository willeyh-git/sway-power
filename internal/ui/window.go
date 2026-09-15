package ui

import (
	"fmt"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"github.com/willeyh-git/sway-power/internal/battery"
	"github.com/willeyh-git/sway-power/internal/config"
)

// Show starts the Sway Power application.
func Show(app fyne.App, cfg config.Config, debug bool) error {
	fmt.Fprintf(os.Stderr, "[ui] starting with debug=%v\n", debug)

	if scale := getWaylandScale(); scale > 0 {
		os.Setenv("FYNE_SCALE", fmt.Sprintf("%.2f", scale))
	}

	// Set custom theme
	app.Settings().SetTheme(CustomTheme())

	window := app.NewWindow("Sway Power")

	// Battery display
	batContent, batDisplay := newBatteryDisplay(cfg)

	// Shared status line
	status := newRichTextLabel("", widget.RichTextStyle{
		SizeName:  SmallSize,
		Alignment: fyne.TextAlignCenter,
	})

	// Power profile buttons
	powerMgr := newPowerProfileManager(debug, cfg, status)

	// Lid close buttons
	lidMgr := newLidCloseButtons(debug, cfg, status)

	// Layout owns all spacing
	layout := NewLayout(batContent, status.Object(), widget.NewSeparator(),
		powerMgr.labelText(), powerMgr.buttonBar(), lidMgr.labelText(), lidMgr.buttonBar())

	window.SetContent(layout.Container())

	// Battery updates
	go func() {
		time.Sleep(100 * time.Millisecond)
		bat, err := battery.Read()
		fyne.Do(func() {
			batDisplay.update(bat, err)
			window.Content().Refresh()
		})

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			bat, err := battery.Read()
			fyne.Do(func() {
				batDisplay.update(bat, err)
			})
		}
	}()

	window.ShowAndRun()
	return nil
}
