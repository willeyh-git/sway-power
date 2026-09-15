package ui

import (
	"fmt"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/willeyh-git/sway-power/internal/battery"
	"github.com/willeyh-git/sway-power/internal/config"
)

// Show starts the Sway Power application.
func Show(app fyne.App, cfg config.Config, debug bool) error {
	fmt.Fprintf(os.Stderr, "[ui] starting with debug=%v\n", debug)

	// Read scale factor from Sway/Wayland.
	if scale := getWaylandScale(); scale > 0 {
		os.Setenv("FYNE_SCALE", fmt.Sprintf("%.2f", scale))
	}

	window := app.NewWindow("Sway Power")
	window.SetFixedSize(true)
	window.Resize(fyne.NewSize(100, 140))

	// Battery display (with EMA smoothing).
	batContent, batDisplay := newBatteryDisplay()

	// Shared status line for one-off messages from the power profile and
	// lid sections; empty (and thus invisible) otherwise.
	status := newRichTextLabel("", widget.RichTextStyle{
		SizeName:  theme.SizeNameCaptionText,
		Alignment: fyne.TextAlignCenter,
	})

	// Power profile buttons.
	powerMgr := newPowerProfileManager(debug, status)

	// Lid close buttons.
	lidMgr := newLidCloseButtons(debug, status)

	content := container.NewVBox(
		batContent,
		status.Object(),
		widget.NewSeparator(),
		powerMgr.buttonBar(),
		lidMgr.buttonBar(),
	)

	window.SetContent(container.NewCenter(content))
	window.Resize(fyne.NewSize(400, 250))

	// Battery updates: first one shortly after mapping, then every 5 seconds.
	go func() {
		time.Sleep(100 * time.Millisecond)

		bat, err := battery.Read()
		fyne.Do(func() {
			batDisplay.update(bat, err)
			window.Content().Refresh() // one forced repaint at the real scale
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
