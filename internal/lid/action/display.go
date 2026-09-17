package action

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// SwaymsgCmd is the swaymsg executor. Override in tests to mock
// swaymsg calls (e.g., avoid calling swaymsg in tests).
var SwaymsgCmd = func(args ...string) *exec.Cmd {
	return exec.Command("swaymsg", args...)
}

// swayOutput mirrors the fields of sway's `get_outputs` JSON that we care
// about.
type swayOutput struct {
	Name      string `json:"name"`
	Interface string `json:"interface"`
	Enabled   bool   `json:"enabled"`
}

// getOutputs queries sway for the current list of outputs.
func getOutputs() ([]swayOutput, error) {
	out, err := SwaymsgCmd("-t", "json", "get_outputs").Output()
	if err != nil {
		return nil, fmt.Errorf("swaymsg get_outputs: %w", err)
	}
	var outputs []swayOutput
	if err := json.Unmarshal(out, &outputs); err != nil {
		return nil, fmt.Errorf("parse get_outputs: %w", err)
	}
	return outputs, nil
}

// isInternalDisplay reports whether the output is the laptop's internal
// display. Sway exposes the kernel interface name (e.g. "eDP-1", "LVDS-1",
// "MIPI-1") which is a reliable way to tell internal panels apart from
// external monitors (HDMI, DisplayPort, USB-C, ...).
func isInternalDisplay(o swayOutput) bool {
	id := o.Interface
	if id == "" {
		id = o.Name
	}
	// "eDP-1" -> "edp", "HDMI-A-1" -> "hdmi"
	id = strings.ToLower(strings.SplitN(id, "-", 2)[0])
	switch id {
	case "edp", "lvds", "lvlvs", "mipi", "eink":
		return true
	}
	return false
}

// outputsToDisable returns the names of internal outputs that should be
// disabled: the internal display is only turned off when at least one
// external monitor is connected (and enabled) itself.
func outputsToDisable(outputs []swayOutput) []string {
	externalConnected := false
	var names []string
	for _, o := range outputs {
		if !o.Enabled {
			continue
		}
		if isInternalDisplay(o) {
			names = append(names, o.Name)
		} else {
			externalConnected = true
		}
	}
	if !externalConnected {
		return nil
	}
	return names
}

// hideInternalDisplay turns off the internal laptop display when an external
// monitor is connected. It returns the names of the outputs it successfully
// disabled (the caller owns restoring them) and an error when a disable
// failed. Returns nil names when nothing had to be done.
func hideInternalDisplay() ([]string, error) {
	outputs, err := getOutputs()
	if err != nil {
		return nil, err
	}

	var disabled []string
	var errs []error
	for _, name := range outputsToDisable(outputs) {
		if err := SwaymsgCmd("output", name, "disable").Run(); err != nil {
			errs = append(errs, fmt.Errorf("disable %s: %w", name, err))
			continue
		}
		disabled = append(disabled, name)
	}
	return disabled, errors.Join(errs...)
}

// ShowInternalDisplay re-enables exactly the outputs sway-power disabled
// (e.g. after the lid was closed with "nothing" selected). Display
// ownership is tracked by the daemon handler, which passes in the names it
// recorded and calls this on lid open — independent of the currently
// configured action. Outputs the user disabled themselves are not in the
// list and are therefore left alone.
func ShowInternalDisplay(names []string) error {
	var errs []error
	for _, name := range names {
		if err := SwaymsgCmd("output", name, "enable").Run(); err != nil {
			errs = append(errs, fmt.Errorf("enable %s: %w", name, err))
		}
	}
	return errors.Join(errs...)
}
