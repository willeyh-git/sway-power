package action

import (
	"fmt"
	"os"
	"os/exec"
)

// Action represents a lid close action.
type Action string

const (
	ActionLock   Action = "lock"
	ActionSleep  Action = "sleep"
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

// Execute runs the appropriate action for the given lid close action.
func (a Action) Execute() error {
	switch a {
	case ActionLock:
		return execSwaylock()
	case ActionSleep:
		return execSystemctl("suspend")
	case ActionNothing:
		return nil
	default:
		return fmt.Errorf("unknown action: %s", a)
	}
}

// execSwaylock runs swaylock to lock the screen.
func execSwaylock() error {
	cmd := exec.Command("swaylock", "-f")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// execSystemctl runs systemctl with the given verb.
func execSystemctl(verb string) error {
	cmd := exec.Command("systemctl", verb)
	return cmd.Run()
}
