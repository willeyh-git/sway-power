package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// myTheme implements fyne.Theme with custom sizes.
type myTheme struct{}

var _ fyne.Theme = (*myTheme)(nil)

func (m *myTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	return theme.DefaultTheme().Color(name, variant)
}

func (m *myTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (m *myTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (m *myTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 0
	case theme.SizeNameInnerPadding:
		return 0
	case theme.SizeNameCaptionText:
		return 8
	case theme.SizeNameHeadingText:
		return 14
	}
	return theme.DefaultTheme().Size(name)
}

// CustomTheme returns a custom theme with 0 padding and custom font sizes.
func CustomTheme() fyne.Theme {
	return &myTheme{}
}
