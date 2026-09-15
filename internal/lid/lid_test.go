package lid

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateString(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{Open, "open"},
		{Closed, "closed"},
		{State(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.state.String(); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadLidState(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		content string
		want    State
		wantErr bool
	}{
		{"open", "open", Open, false},
		{"closed", "closed", Closed, false},
		{"open upper", "OPEN", Open, false},
		{"closed upper", "CLOSED", Closed, false},
		{"unknown", "foobar", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(tmpDir, "state")
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatalf("setup: failed to write file: %v", err)
			}

			got, err := readLidState(path)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindLidState(t *testing.T) {
	// findLidState tries real paths, so we just verify it returns something
	// or nothing — we can't easily mock this without changing the implementation.
	got := findLidState()
	if got != "" {
		t.Logf("found lid state at %s", got)
	}
	// It's OK if we don't find one (e.g., on a desktop without ACPI lid)
}
