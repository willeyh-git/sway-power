package action

import (
	"testing"
)

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
		if err := a.Execute(); err == nil {
			t.Fatal("expected error for invalid action")
		}
	})
}
