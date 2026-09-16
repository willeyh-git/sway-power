package daemon

import (
	"sync"

	"github.com/willeyh-git/sway-power/internal/lid/action"
)

// Handler manages the current lid action and executes it on lid events.
type Handler struct {
	mu      sync.RWMutex
	current action.Action
	log     Logger
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
	if err := a.Execute(); err != nil {
		h.log.Printf("handler: failed to execute %q: %v", a, err)
	}
}

// HandleLidOpen handles lid open events.
func (h *Handler) HandleLidOpen() {
	h.mu.RLock()
	a := h.current
	h.mu.RUnlock()

	h.log.Printf("handler: handling lid open for action %q", a)
	if err := a.OnOpen(); err != nil {
		h.log.Printf("handler: failed to handle open for %q: %v", a, err)
	}
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
