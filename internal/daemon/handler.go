package daemon

import (
	"sync"

	"github.com/willeyh-git/sway-power/internal/lid/action"
)

// Handler manages the current lid action and executes it on lid events.
type Handler struct {
	mu      sync.RWMutex
	current action.Action
	// disabledByUs records exactly which outputs sway-power disabled on lid
	// close (action "nothing"). While non-empty, the next lid-open restores
	// those outputs — and only those, never outputs the user disabled
	// themselves — regardless of the currently configured action: the user
	// may have changed e.g. nothing → lock/sleep while the lid was closed.
	// Entries are removed once their restore succeeds.
	disabledByUs map[string]bool
	log            Logger
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
	disabled, err := a.Execute()
	if err != nil {
		h.log.Printf("handler: failed to execute %q: %v", a, err)
	}
	// Ownership only covers outputs that were actually disabled, so a
	// failed Execute (e.g. swaymsg down) never claims displays we didn't
	// turn off. Previously disabled but unrestored outputs stay in the
	// map: they are still ours to restore.
	if len(disabled) > 0 {
		h.mu.Lock()
		if h.disabledByUs == nil {
			h.disabledByUs = make(map[string]bool)
		}
		for _, name := range disabled {
			h.disabledByUs[name] = true
		}
		h.mu.Unlock()
	}
}

// HandleLidOpen handles lid open events.
func (h *Handler) HandleLidOpen() {
	h.mu.RLock()
	a := h.current
	toRestore := make([]string, 0, len(h.disabledByUs))
	for name := range h.disabledByUs {
		toRestore = append(toRestore, name)
	}
	h.mu.RUnlock()

	h.log.Printf("handler: handling lid open for action %q", a)
	if len(toRestore) > 0 {
		// Restore even if the action changed since the lid close (e.g.
		// nothing → lock/sleep): display ownership is tracked separately
		// from the current action.
		if err := action.ShowInternalDisplay(toRestore); err != nil {
			h.log.Printf("handler: failed to restore displays %v: %v (will retry on next lid open)", toRestore, err)
			// Keep the entries: retry on the next lid-open event.
		} else {
			h.mu.Lock()
			for _, name := range toRestore {
				delete(h.disabledByUs, name)
			}
			h.mu.Unlock()
		}
	}
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
