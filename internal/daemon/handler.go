package daemon

import (
	"sync"

	"github.com/willeyh-git/sway-power/internal/lid/action"
)

// Handler manages the current lid action and executes it on lid events.
type Handler struct {
	mu      sync.RWMutex
	current action.Action
	// internalDisplayDisabledByUs records that we disabled the internal
	// display on lid close (action "nothing"). While it is set, the next
	// lid-open restores the display regardless of the currently configured
	// action — the user may have changed e.g. nothing → lock/sleep while the
	// lid was closed. It is cleared once the restore succeeds.
	internalDisplayDisabledByUs bool
	log                         Logger
}

// NewHandler creates a new Handler with the given initial action.
func NewHandler(initial action.Action, log Logger) *Handler {
	if err := initial.Validate(); err != nil {
		log.Printf("handler: invalid initial action %q: %v", initial, err)
		// Fall back to lock.
		initial = action.ActionLock
	}
	return &Handler{
		current: initial,
		log:     log,
	}
}

// HandleLidClosed executes the current action on lid close.
func (h *Handler) HandleLidClosed() {
	h.mu.RLock()
	a := h.current
	h.mu.RUnlock()

	h.log.Printf("handler: executing action %q on lid close", a)
	if a == action.ActionNothing {
		// The internal display is ours to restore on the next lid-open,
		// regardless of any action change while the lid stays closed. Set
		// even if Execute later fails: the disable may have partially
		// succeeded.
		h.setInternalDisplayDisabledByUs(true)
	}
	if err := a.Execute(); err != nil {
		h.log.Printf("handler: failed to execute %q: %v", a, err)
	}
}

// HandleLidOpen handles lid open events.
func (h *Handler) HandleLidOpen() {
	h.mu.RLock()
	a := h.current
	owned := h.internalDisplayDisabledByUs
	h.mu.RUnlock()

	h.log.Printf("handler: handling lid open for action %q", a)
	if owned {
		// Restore the internal display even if the action changed since the
		// lid close (e.g. nothing → lock/sleep): display ownership is tracked
		// separately from the current action.
		if err := action.ShowInternalDisplay(); err != nil {
			h.log.Printf("handler: failed to restore internal display: %v", err)
			// Keep the flag set: retry on the next lid-open event.
		} else {
			h.setInternalDisplayDisabledByUs(false)
		}
	}
	if err := a.OnOpen(); err != nil {
		h.log.Printf("handler: failed to handle open for %q: %v", a, err)
	}
}

func (h *Handler) setInternalDisplayDisabledByUs(owned bool) {
	h.mu.Lock()
	h.internalDisplayDisabledByUs = owned
	h.mu.Unlock()
}

// SetAction atomically swaps to a new action.
func (h *Handler) SetAction(a action.Action) {
	if err := a.Validate(); err != nil {
		h.log.Printf("handler: invalid action %q, keeping %q: %v", a, h.current, err)
		return
	}
	h.mu.Lock()
	if h.current != a {
		h.log.Printf("handler: action swapped: %q → %q", h.current, a)
		h.current = a
	}
	h.mu.Unlock()
}

// GetAction returns the current action (read-only).
func (h *Handler) GetAction() action.Action {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.current
}
