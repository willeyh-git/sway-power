package action

import (
	"os/exec"
	"testing"
)

func TestOnOpenDoesNotTouchDisplays(t *testing.T) {
	// Re-enabling the internal display is owned by the daemon handler
	// (internalDisplayDisabledByUs), not by the actions — so no action may
	// query sway outputs on lid open.
	for _, a := range []Action{ActionLock, ActionSleep, ActionNothing} {
		t.Run(string(a), func(t *testing.T) {
			called := false
			orig := SwaymsgCmd
			SwaymsgCmd = func(args ...string) *exec.Cmd {
				called = true
				return exec.Command("true")
			}
			t.Cleanup(func() { SwaymsgCmd = orig })

			if err := a.OnOpen(); err != nil {
				t.Fatalf("OnOpen() = %v, want nil", err)
			}
			if called {
				t.Error("OnOpen must not query sway outputs")
			}
		})
	}
}

func TestActionValidate(t *testing.T) {
	valid := []Action{
		ActionLock, ActionSleep, ActionNothing,
	}

	for _, a := range valid {
		t.Run(string(a), func(t *testing.T) {
			if err := a.Validate(); err != nil {
				t.Fatalf("expected no error for %q, got %v", a, err)
			}
		})
	}

	t.Run("invalid action", func(t *testing.T) {
		a := Action("invalid")
		if err := a.Validate(); err == nil {
			t.Fatal("expected error for invalid action")
		}
	})
}

func TestActionExecute(t *testing.T) {
	// Execute calls external commands, so we just verify the method exists
	// and returns an error for invalid actions.
	t.Run("invalid action returns error", func(t *testing.T) {
		a := Action("invalid")
		if _, err := a.Execute(); err == nil {
			t.Fatal("expected error for invalid action")
		}
	})
}
