// Package action implements actions executed on lid open/close events.
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
//
// Re-enabling the internal display is deliberately NOT done here: the daemon
// handler tracks whether sway-power disabled it (internalDisplayDisabledByUs)
// and restores it on lid open regardless of the current action. That keeps
// display ownership independent of the action — e.g. if the user switches
// nothing → lock/sleep while the lid is closed, the panel is still restored
// on open.
func (a Action) OnOpen() error {
	return nil
}

// ExecFunc is a function signature for executing commands.
type ExecFunc func(name string, args ...string) error

// Exec is the package-level command executor. Override in tests to mock
// command execution (e.g., avoid running swaylock or systemctl).
var Exec ExecFunc = realExec

// realExec is the default implementation that runs commands via os/exec.
func realExec(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	return cmd.Run()
}

// Swaylock runs swaylock. Override in tests to mock the lock action
// (consistent with Exec and SwaymsgCmd, so no real lock screen is
// launched in tests).
var Swaylock = realSwaylock

// realSwaylock runs `swaylock -f`. swaylock -f daemonizes, so Start
// returns once the lock is running and we do not need to hold onto it.
func realSwaylock(args ...string) error {
	cmd := exec.Command("swaylock", args...)
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

// execSwaylock runs swaylock to lock the screen.
func execSwaylock() error {
	return Swaylock("-f")
}

// execSystemctl runs systemctl with the given verb.
func execSystemctl(verb string) error {
	return Exec("systemctl", verb)
}
