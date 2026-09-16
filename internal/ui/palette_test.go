package ui

import (
	"image/color"
	"testing"

	"github.com/willeyh-git/sway-power/internal/config"
)

func TestBuildPaletteLightDefaults(t *testing.T) {
	pal := BuildPalette(config.Default(), false)

	if pal.Mode != "light" {
		t.Fatalf("expected light mode, got %q", pal.Mode)
	}

	checks := []struct {
		name string
		got  color.NRGBA
		want string
	}{
		{"accent", pal.Accent, "#3584E4"},
		{"background", pal.Background, "#F6F5F4"},
		{"track", pal.Track, "#DEDDDA"},
		{"label", pal.Label, "#77767B"},
		{"value", pal.Value, "#5E5C64"},
		{"category", pal.Category, "#3D3846"},
		{"title", pal.Title, "#241F31"},
		{"button", pal.Button, "#DEDDDA"},
		{"buttonLabel", pal.ButtonLabel, "#5E5C64"},
	}
	for _, c := range checks {
		if c.got != parseHexColor(c.want) {
			t.Errorf("%s: got %v, want %v", c.name, c.got, parseHexColor(c.want))
		}
	}

	// Accent-driven defaults.
	if pal.Icon != pal.Accent {
		t.Errorf("icon should default to accent, got %v", pal.Icon)
	}
	if pal.ButtonBorder != pal.Accent || pal.ButtonHover != pal.Accent || pal.ButtonActive != pal.Accent {
		t.Errorf("border/hover/active should default to accent")
	}

	// White text on the blue accent.
	white := color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	if pal.OnActive != white {
		t.Errorf("OnActive should be white, got %v", pal.OnActive)
	}
}

func TestBuildPaletteDarkDefaults(t *testing.T) {
	pal := BuildPalette(config.Default(), true)

	if pal.Mode != "dark" {
		t.Fatalf("expected dark mode, got %q", pal.Mode)
	}

	checks := []struct {
		name string
		got  color.NRGBA
		want string
	}{
		{"background", pal.Background, "#3D3846"},
		{"track", pal.Track, "#5E5C64"},
		{"label", pal.Label, "#9A9996"},
		{"value", pal.Value, "#C0BFBC"},
		{"category", pal.Category, "#DEDDDA"},
		{"title", pal.Title, "#F6F5F4"},
		{"button", pal.Button, "#5E5C64"},
		{"buttonLabel", pal.ButtonLabel, "#C0BFBC"},
	}
	for _, c := range checks {
		if c.got != parseHexColor(c.want) {
			t.Errorf("%s: got %v, want %v", c.name, c.got, parseHexColor(c.want))
		}
	}
}

func TestBuildPaletteUserOverrideTakesPrecedence(t *testing.T) {
	cfg := config.Default()
	cfg.UI.Accent = "#FF7800" // Orange 3

	pal := BuildPalette(cfg, false)

	if pal.Accent != parseHexColor("#FF7800") {
		t.Fatalf("user accent not applied, got %v", pal.Accent)
	}

	// Everything accent-derived follows the user accent.
	if pal.Icon != pal.Accent || pal.ButtonBorder != pal.Accent ||
		pal.ButtonHover != pal.Accent || pal.ButtonActive != pal.Accent {
		t.Errorf("accent-derived colors should follow user accent")
	}

	// Non-accent colors keep their fallbacks.
	if pal.Background != parseHexColor("#F6F5F4") {
		t.Errorf("background should keep fallback, got %v", pal.Background)
	}

	// Orange is light: text on it must be black.
	black := color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xFF}
	if pal.OnActive != black {
		t.Errorf("OnActive on orange should be black, got %v", pal.OnActive)
	}
}

func TestBuildPaletteExplicitModeOverridesSystem(t *testing.T) {
	cfg := config.Default()
	cfg.UI.Mode = "dark"
	if pal := BuildPalette(cfg, false); pal.Mode != "dark" {
		t.Errorf("explicit dark not honored, got %q", pal.Mode)
	}

	cfg.UI.Mode = "light"
	if pal := BuildPalette(cfg, true); pal.Mode != "light" {
		t.Errorf("explicit light not honored, got %q", pal.Mode)
	}
}
