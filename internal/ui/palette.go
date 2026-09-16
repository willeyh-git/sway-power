package ui

import (
	"image/color"
	"strings"

	"github.com/willeyh-git/sway-power/internal/config"
)

// Advaita palette (GNOME design language).
// See https://design.gnome.org/assets/reference/advaita-colors.html
const (
	// Blue 3 — the accent.
	advaitaBlue3 = "#3584E4"

	// Light neutrals.
	advaitaLight1 = "#FFFFFF"
	advaitaLight2 = "#F6F5F4"
	advaitaLight3 = "#DEDDDA"
	advaitaLight4 = "#C0BFBC"
	advaitaLight5 = "#9A9996"

	// Dark neutrals.
	advaitaDark1 = "#77767B"
	advaitaDark2 = "#5E5C64"
	advaitaDark3 = "#3D3846"
	advaitaDark4 = "#241F31"
	advaitaDark5 = "#000000"

	// Status colors.
	advaitaGreen3  = "#33D17A"
	advaitaYellow4 = "#F5C211"
	advaitaRed3    = "#E01B24"
)

// Palette is the fully resolved color set for the running theme.
// Every field has a concrete value; nothing in the UI may fall back to
// hardcoded colors.
type Palette struct {
	Mode string // "light" or "dark"

	Accent     color.NRGBA // default: Blue 3
	Background color.NRGBA // window background
	Track      color.NRGBA // battery track background; fill is Accent
	Label      color.NRGBA // muted text
	Value      color.NRGBA // emphasized text (more contrast than Label)
	Category   color.NRGBA // section headings ("Power Profile", "Lid Settings")
	Title      color.NRGBA // strongest text ("Battery")
	Icon       color.NRGBA // battery icon; default: Accent

	Button       color.NRGBA // unselected button background
	ButtonLabel  color.NRGBA // unselected button text
	ButtonBorder color.NRGBA // selected button border; default: Accent
	ButtonHover  color.NRGBA // hover/focus background; default: Accent
	ButtonActive color.NRGBA // selected background; default: Accent

	OnHover  color.NRGBA // readable text on ButtonHover (auto contrast)
	OnActive color.NRGBA // readable text on ButtonActive (auto contrast)

	Success color.NRGBA // Green 3
	Warning color.NRGBA // Yellow 4
	Error   color.NRGBA // Red 3
}

// BuildPalette resolves the effective palette: each user-provided config
// color takes precedence; anything unset falls back, and accent-driven
// defaults (icon, border, hover, active, track fill) follow the effective
// accent even when the user only overrides ui.accent.
func BuildPalette(cfg config.Config, sysDark bool) Palette {
	mode := "light"
	switch strings.ToLower(strings.TrimSpace(cfg.UI.Mode)) {
	case "dark":
		mode = "dark"
	case "light":
		mode = "light"
	default: // auto
		if sysDark {
			mode = "dark"
		}
	}

	pal := Palette{Mode: mode}

	pal.Accent = pick(cfg.UI.Accent, parseHexColor(advaitaBlue3))
	pal.Icon = pick(cfg.UI.Icon, pal.Accent)
	pal.ButtonBorder = pick(cfg.UI.ButtonBorder, pal.Accent)
	pal.ButtonHover = pick(cfg.UI.ButtonHover, pal.Accent)
	pal.ButtonActive = pick(cfg.UI.ButtonActive, pal.Accent)

	if mode == "light" {
		// Light: light background, dark text; values darker than labels.
		pal.Background = pick(cfg.UI.Background, parseHexColor(advaitaLight2))
		pal.Track = pick(cfg.UI.Track, parseHexColor(advaitaLight3))
		pal.Label = pick(cfg.UI.Label, parseHexColor(advaitaDark1))
		pal.Value = pick(cfg.UI.Value, parseHexColor(advaitaDark2))
		pal.Category = pick(cfg.UI.Category, parseHexColor(advaitaDark3))
		pal.Title = pick(cfg.UI.Title, parseHexColor(advaitaDark4))
		pal.Button = pick(cfg.UI.Button, parseHexColor(advaitaLight3))
		pal.ButtonLabel = pick(cfg.UI.ButtonLabel, parseHexColor(advaitaDark2))
	} else {
		// Dark: dark background, light text; values lighter than labels.
		pal.Background = pick(cfg.UI.Background, parseHexColor(advaitaDark3))
		pal.Track = pick(cfg.UI.Track, parseHexColor(advaitaDark2))
		pal.Label = pick(cfg.UI.Label, parseHexColor(advaitaLight5))
		pal.Value = pick(cfg.UI.Value, parseHexColor(advaitaLight4))
		pal.Category = pick(cfg.UI.Category, parseHexColor(advaitaLight3))
		pal.Title = pick(cfg.UI.Title, parseHexColor(advaitaLight2))
		pal.Button = pick(cfg.UI.Button, parseHexColor(advaitaDark2))
		pal.ButtonLabel = pick(cfg.UI.ButtonLabel, parseHexColor(advaitaLight4))
	}

	pal.OnHover = onColor(pal.ButtonHover)
	pal.OnActive = onColor(pal.ButtonActive)

	pal.Success = parseHexColor(advaitaGreen3)
	pal.Warning = parseHexColor(advaitaYellow4)
	pal.Error = parseHexColor(advaitaRed3)

	return pal
}

// pick returns the user value as a color if set, otherwise def.
func pick(userValue string, def color.NRGBA) color.NRGBA {
	if userValue == "" {
		return def
	}
	return parseHexColor(userValue)
}

// onColor returns a text color that is readable on bg (white or black).
func onColor(bg color.NRGBA) color.NRGBA {
	luminance := 0.299*float64(bg.R) + 0.587*float64(bg.G) + 0.114*float64(bg.B)
	if luminance < 140 {
		return color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	}
	return color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xFF}
}
