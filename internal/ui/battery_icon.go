package ui

import _ "embed"

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
)

//go:embed battery.svg
var batteryIconSVG []byte

// newBatteryIcon returns a battery icon tinted with the theme's
// foreground color.
func newBatteryIcon() *canvas.Image {
	res := fyne.NewStaticResource("battery.svg", batteryIconSVG)
	themed := theme.NewThemedResource(res)

	img := canvas.NewImageFromResource(themed)
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(28, 28))
	return img
}
