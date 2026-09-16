package config

import (
	"strings"
	"testing"
)

func TestUIColorsValidate(t *testing.T) {
	valid := UIColors{
		Mode:         "auto",
		Accent:       "#3584E4",
		Background:   "#F6F5F4",
		Track:        "#DEDDDA",
		Label:        "#77767B",
		Value:        "#5E5C64",
		Category:     "#3D3846",
		Title:        "#241F31",
		Icon:         "3584e4",
		Button:       "#DEDDDA",
		ButtonLabel:  "#5E5C64",
		ButtonBorder: "#3584E4",
		ButtonHover:  "#3584E4",
		ButtonActive: "#3584E4",
	}

	t.Run("defaults are valid", func(t *testing.T) {
		if err := valid.Validate(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("empty values are valid", func(t *testing.T) {
		var empty UIColors
		if err := empty.Validate(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("accepts colors without '#' prefix", func(t *testing.T) {
		c := valid
		c.Value = "5E5C64"
		if err := c.Validate(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("accepts uppercase hex", func(t *testing.T) {
		c := valid
		c.Category = "#DEDDDA"
		if err := c.Validate(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	invalid := []string{
		"#ff00",       // too short
		"#ff00001",    // too long
		"#gggggg",     // not hex
		"not a color", // nonsense
		"#ff00g8",     // mixed
	}

	for _, v := range invalid {
		c := valid
		c.ButtonLabel = v

		t.Run("rejects "+v, func(t *testing.T) {
			err := c.Validate()
			if err == nil {
				t.Fatalf("expected error for %q", v)
			}

			want := `ui.button_label: invalid color "` + v + `"`
			if err.Error() != want {
				t.Fatalf("unexpected error format:\n got  %q\n want %q", err.Error(), want)
			}
		})
	}
}

func TestModeValidate(t *testing.T) {
	t.Run("valid modes", func(t *testing.T) {
		for _, mode := range []string{"", "auto", "light", "dark"} {
			u := UIColors{Mode: mode}
			if err := u.Validate(); err != nil {
				t.Fatalf("expected no error for mode %q, got %v", mode, err)
			}
		}
	})

	t.Run("rejects invalid mode", func(t *testing.T) {
		u := UIColors{Mode: "blue"}
		err := u.Validate()
		if err == nil {
			t.Fatal("expected error")
		}
		want := `ui.mode: invalid mode "blue" (valid: auto, light, dark)`
		if err.Error() != want {
			t.Fatalf("got %q, want %q", err.Error(), want)
		}
	})
}

func TestConfigValidateReportsField(t *testing.T) {
	cfg := Default()
	cfg.UI.Track = "#nope"

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error")
	}

	if got, want := err.Error(), `ui.track: invalid color "#nope"`; !strings.Contains(err.Error(), want) {
		t.Fatalf("got %q, want substring %q", got, want)
	}
}

func TestLidCloseValidate(t *testing.T) {
	validActions := []string{"lock", "sleep", "nothing"}
	for _, action := range validActions {
		t.Run("valid "+action, func(t *testing.T) {
			lc := LidClose{Action: action}
			if err := lc.Validate(); err != nil {
				t.Fatalf("expected no error for %q, got %v", action, err)
			}
		})
	}

	t.Run("empty action is valid", func(t *testing.T) {
		lc := LidClose{}
		if err := lc.Validate(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("rejects invalid action", func(t *testing.T) {
		lc := LidClose{Action: "invalid"}
		err := lc.Validate()
		if err == nil {
			t.Fatal("expected error")
		}
		if got := err.Error(); !strings.Contains(got, `lid_close.action: invalid action "invalid"`) {
			t.Fatalf("got %q, want substring %q", got, `invalid action "invalid"`)
		}
	})
}
