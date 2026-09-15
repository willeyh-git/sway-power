package action

import (
	"testing"
)

func TestIsInternalDisplay(t *testing.T) {
	tests := []struct {
		output swayOutput
		want   bool
	}{
		{swayOutput{Name: "eDP-1", Interface: "eDP-1"}, true},
		{swayOutput{Name: "LVDS-1", Interface: "LVDS-1"}, true},
		{swayOutput{Name: "MIPI-1", Interface: "MIPI-1"}, true},
		{swayOutput{Name: "HDMI-A-1", Interface: "HDMI-A-1"}, false},
		{swayOutput{Name: "DP-1", Interface: "DP-1"}, false},
		{swayOutput{Name: "USBC-1", Interface: "USB-C-1"}, false},
		{swayOutput{Name: "VGA-1", Interface: "VGA-1"}, false},
		// No interface field (old sway): fall back to the name.
		{swayOutput{Name: "eDP-1"}, true},
		{swayOutput{Name: "HDMI-A-1"}, false},
	}

	for _, tt := range tests {
		if got := isInternalDisplay(tt.output); got != tt.want {
			t.Errorf("isInternalDisplay(%q) = %v, want %v", tt.output.Name, got, tt.want)
		}
	}
}

func TestOutputsToDisable(t *testing.T) {
	internal := swayOutput{Name: "eDP-1", Interface: "eDP-1", Enabled: true}
	external := swayOutput{Name: "HDMI-A-1", Interface: "HDMI-A-1", Enabled: true}
	externalOff := swayOutput{Name: "DP-1", Interface: "DP-1", Enabled: false}

	t.Run("external connected: disable internal", func(t *testing.T) {
		got := outputsToDisable([]swayOutput{internal, external})
		if len(got) != 1 || got[0] != "eDP-1" {
			t.Fatalf("got %v, want [eDP-1]", got)
		}
	})

	t.Run("no external: leave internal on", func(t *testing.T) {
		if got := outputsToDisable([]swayOutput{internal}); got != nil {
			t.Fatalf("got %v, want nil", got)
		}
	})

	t.Run("external present but not enabled: leave internal on", func(t *testing.T) {
		if got := outputsToDisable([]swayOutput{internal, externalOff}); got != nil {
			t.Fatalf("got %v, want nil", got)
		}
	})

	t.Run("internal already off: nothing to disable", func(t *testing.T) {
		off := internal
		off.Enabled = false
		if got := outputsToDisable([]swayOutput{off, external}); got != nil {
			t.Fatalf("got %v, want nil", got)
		}
	})

	t.Run("no outputs: nothing to disable", func(t *testing.T) {
		if got := outputsToDisable(nil); got != nil {
			t.Fatalf("got %v, want nil", got)
		}
	})
}
