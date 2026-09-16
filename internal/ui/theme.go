package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Custom theme color names for our text roles. RichText segments reference
// these via RichTextStyle.ColorName, so every text object resolves through
// the app theme and therefore honors the user config.
const (
	themeNameLabel    fyne.ThemeColorName = "sway-power.label"
	themeNameValue    fyne.ThemeColorName = "sway-power.value"
	themeNameCategory fyne.ThemeColorName = "sway-power.category"
	themeNameTitle    fyne.ThemeColorName = "sway-power.title"
)

// appTheme implements fyne.Theme using the resolved app palette, so theme
// rendered widgets (rich text, menus, dialogs, separators, ...) honor the
// user config.
type appTheme struct {
	pal Palette
}

var _ fyne.Theme = (*appTheme)(nil)

func (t *appTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground, theme.ColorNameMenuBackground:
		return t.pal.Background
	case theme.ColorNameForeground, themeNameTitle:
		return t.pal.Title
	case themeNameLabel, theme.ColorNamePlaceHolder, theme.ColorNameDisabled:
		return t.pal.Label
	case themeNameValue:
		return t.pal.Value
	case theme.ColorNameHyperlink:
		return t.pal.Accent
	case themeNameCategory:
		return t.pal.Category
	case theme.ColorNameButton:
		return t.pal.Button
	case theme.ColorNameHover:
		return t.pal.ButtonHover
	case theme.ColorNameDisabledButton, theme.ColorNameInputBackground,
		theme.ColorNameSeparator, theme.ColorNameScrollBarBackground, theme.ColorNameScrollBar:
		return t.pal.Track
	case theme.ColorNamePrimary, theme.ColorNameSelection, theme.ColorNameFocus:
		return t.pal.Accent
	case theme.ColorNameForegroundOnPrimary:
		return t.pal.OnActive
	case theme.ColorNameSuccess:
		return t.pal.Success
	case theme.ColorNameWarning:
		return t.pal.Warning
	case theme.ColorNameError:
		return t.pal.Error
	}

	// Fall back to the default theme for anything we don't map.
	// Keep the forced-mode behavior: a light palette must never render
	// dark defaults (or vice versa).
	if t.pal.Mode == "light" && variant == theme.VariantDark {
		variant = theme.VariantLight
	}
	return theme.DefaultTheme().Color(name, variant)
}

func (t *appTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (t *appTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (t *appTheme) Size(name fyne.ThemeSizeName) float32 {
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

// NewTheme returns a fyne.Theme backed by pal.
func NewTheme(pal Palette) fyne.Theme {
	return &appTheme{pal: pal}
}
