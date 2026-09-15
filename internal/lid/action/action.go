package action

import (
	"fmt"
	"os"
	"os/exec"
)

// Action represents a lid close action.
type Action string

const (
	ActionLock    Action = "lock"
	ActionSleep   Action = "sleep"
	ActionNothing Action = "nothing"
)

// Validate checks that the action is valid.
func (a Action) Validate() error {
	switch a {
	case ActionLock, ActionSleep, ActionNothing:
		return nil
	default:
		return fmt.Errorf("invalid action %q (valid: lock, sleep, nothing)", a)
	}
}

// Execute runs the action for a lid close event.
// The compositor (swayidle) does not listen to logind's lock-session D-Bus
// signals, so "lock" runs swaylock manually instead of relying on logind.
// "nothing" does not suspend; instead the internal display is turned off
// when an external monitor is connected so the machine stays usable.
func (a Action) Execute() error {
	switch a {
	case ActionLock:
		return execSwaylock()
	case ActionSleep:
		return execSystemctl("suspend")
	case ActionNothing:
		return hideInternalDisplay()
	default:
		return fmt.Errorf("unknown action: %s", a)
	}
}

// OnOpen runs the action for a lid open event.
// For "nothing" the internal display is re-enabled if it was turned off.
func (a Action) OnOpen() error {
	switch a {
	case ActionNothing:
		return showInternalDisplay()
	default:
		return nil
	}
}

// execSwaylock runs swaylock to lock the screen.
// swaylock -f daemonizes, so Start returns once the lock is running and we
// do not need to hold onto it.
func execSwaylock() error {
	cmd := exec.Command("swaylock", "-f")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start swaylock: %w", err)
	}
	// Reap once the lock goes away; sway-power should not leak zombies.
	go cmd.Wait()
	return nil
}

// execSystemctl runs systemctl with the given verb.
func execSystemctl(verb string) error {
	cmd := exec.Command("systemctl", verb)
	return cmd.Run()
}
