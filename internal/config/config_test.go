package config

import (
	"strings"
	"testing"
)

func TestColorsValidate(t *testing.T) {
	valid := Colors{
		Track:    "#2d2d32",
		Normal:   "#50c878",
		Charging: "#50aaff",
		Warning:  "#f0b43c",
		Critical: "#e64646",
	}

	t.Run("defaults are valid", func(t *testing.T) {
		if err := valid.Validate(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("empty values are valid", func(t *testing.T) {
		var empty Colors
		if err := empty.Validate(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("accepts colors without '#' prefix", func(t *testing.T) {
		c := valid
		c.Normal = "50c878"
		if err := c.Validate(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("accepts uppercase hex", func(t *testing.T) {
		c := valid
		c.Warning = "#F0B43C"
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
		"",            // handled above, but also check explicit
	}

	for _, v := range invalid {
		if v == "" {
			continue
		}

		c := valid
		c.Critical = v

		t.Run("rejects "+v, func(t *testing.T) {
			err := c.Validate()
			if err == nil {
				t.Fatalf("expected error for %q", v)
			}

			want := `colors.critical: invalid color "` + v + `"`
			if err.Error() != want {
				t.Fatalf("unexpected error format:\n got  %q\n want %q", err.Error(), want)
			}
		})
	}
}

func TestConfigValidateReportsField(t *testing.T) {
	cfg := Default()
	cfg.Colors.Track = "#nope"

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error")
	}

	if got, want := err.Error(), `colors.track: invalid color "#nope"`; !strings.Contains(err.Error(), want) {
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
			t.Fatalf("expected no error for empty action, got %v", err)
		}
	})

	t.Run("rejects invalid action", func(t *testing.T) {
		lc := LidClose{Action: "invalid"}
		err := lc.Validate()
		if err == nil {
			t.Fatal("expected error for invalid action")
		}
		if got := err.Error(); !strings.Contains(got, `lid_close.action: invalid action "invalid"`) {
			t.Fatalf("got %q, want substring %q", got, `invalid action "invalid"`)
		}
	})
}
