package daemon

import (
	"os"
	"path/filepath"
	"time"

	"github.com/willeyh-git/sway-power/internal/lid/action"
	"github.com/willeyh-git/sway-power/internal/preferences"
)

// PreferencesWatcher polls preferences.json for mtime changes and
// notifies the handler when the action setting changes.
type PreferencesWatcher struct {
	prefsPath string
	handler   *Handler
	log       Logger
	stopCh    chan struct{}
	done      chan struct{}
}

// NewPreferencesWatcher creates a watcher that polls preferences.json every
// second for mtime changes.
func NewPreferencesWatcher(handler *Handler, log Logger) *PreferencesWatcher {
	configDir, err := os.UserConfigDir()
	if err != nil {
		log.Printf("prefs: could not find config dir: %v", err)
		return nil
	}

	pw := &PreferencesWatcher{
		prefsPath: filepath.Join(configDir, "sway-power", "preferences.json"),
		handler:   handler,
		log:       log,
		stopCh:    make(chan struct{}),
		done:      make(chan struct{}),
	}

	// Load current action to initialize handler.
	prefs, err := preferences.LoadFromPath(pw.prefsPath)
	if err != nil {
		log.Printf("prefs: initial load failed: %v (using default)", err)
		prefs = preferences.Default()
	}
	handler.SetAction(action.Action(prefs.LidClose))

	go pw.watch()
	return pw
}

// watch polls for mtime changes and swaps the action.
func (pw *PreferencesWatcher) watch() {
	defer close(pw.done)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var lastMod time.Time

	for {
		select {
		case <-pw.stopCh:
			return
		case <-ticker.C:
			info, err := os.Stat(pw.prefsPath)
			if err != nil {
				if os.IsNotExist(err) {
					// File doesn't exist yet; use default.
					continue
				}
				pw.log.Printf("prefs: stat error: %v", err)
				continue
			}

			modTime := info.ModTime()
			if modTime.Equal(lastMod) {
				continue // no change
			}
			lastMod = modTime

			// File changed; load and validate.
			prefs, err := preferences.LoadFromPath(pw.prefsPath)
			if err != nil {
				pw.log.Printf("prefs: load failed: %v (keeping current)", err)
				continue
			}

			newAction := action.Action(prefs.LidClose)
			if err := newAction.Validate(); err != nil {
				pw.log.Printf("prefs: invalid action %q (keeping current): %v", prefs.LidClose, err)
				continue
			}

			pw.handler.SetAction(newAction)
		}
	}
}

// Stop stops the watcher.
func (pw *PreferencesWatcher) Stop() {
	select {
	case <-pw.stopCh:
		return
	default:
	}
	close(pw.stopCh)
	<-pw.done
}
