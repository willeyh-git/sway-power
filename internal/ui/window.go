package ui

import (
	"fmt"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
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

	// Resolve the palette: user colors take precedence, everything else
	// falls back to the Advaita-based light/dark defaults.
	sysDark := app.Settings().ThemeVariant() == theme.VariantDark
	pal := BuildPalette(cfg, sysDark)

	// Set custom theme
	app.Settings().SetTheme(NewTheme(pal))

	window := app.NewWindow("Sway Power")

	// Battery display
	batContent, batDisplay := newBatteryDisplay(pal)

	// Shared status line
	status := newRichTextLabel("", widget.RichTextStyle{
		SizeName:  SmallSize,
		Alignment: fyne.TextAlignCenter,
		ColorName: themeNameValue,
	})

	// Power profile buttons
	powerMgr := newPowerProfileManager(debug, pal, status)

	// Lid close buttons
	lidMgr := newLidCloseButtons(debug, pal, status)

	// Layout owns all spacing
	layout := NewLayout(batContent, status.Object(), widget.NewSeparator(),
		powerMgr.labelText(), powerMgr.buttonBar(), lidMgr.labelText(), lidMgr.buttonBar())

	// Set content with background
	bg := canvas.NewRectangle(pal.Background)
	bg.Resize(fyne.NewSize(400, 180))
	content := layout.Container()
	window.SetContent(container.NewStack(bg, content))

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
