package config

import (
	"context"
	"path/filepath"
	"time"

	"github.com/Nash0810/gobalance/internal/logging"
	"github.com/fsnotify/fsnotify"
)

// Watcher watches for config file changes and triggers reloads
type Watcher struct {
	filepath string
	logger   *logging.Logger
	onChange func(*Config) error
	watcher  *fsnotify.Watcher
}

// NewWatcher creates a new config file watcher
func NewWatcher(filepath string, logger *logging.Logger, onChange func(*Config) error) (*Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	// Watch the directory containing the config file (handles editor atomic writes)
	dir := filepath
	if !isDir(filepath) {
		dir = filepath[:len(filepath)-len(filepath[len(filepath)-1:])]
	}

	if err := watcher.Add(dir); err != nil {
		watcher.Close()
		return nil, err
	}

	return &Watcher{
		filepath: filepath,
		logger:   logger,
		onChange: onChange,
		watcher:  watcher,
	}, nil
}

// Start begins watching for config changes
func (w *Watcher) Start(ctx context.Context) {
	w.logger.Info("config_watcher_started", "file", w.filepath)

	// Debounce timer to avoid multiple reloads
	var debounceTimer *time.Timer
	debounceDuration := 500 * time.Millisecond

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("config_watcher_stopped")
			w.watcher.Close()
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			// Only reload on Write or Create events for our config file
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				// Check if this is our config file (compare base names)
				if filepath.Base(event.Name) == filepath.Base(w.filepath) {
					w.logger.Info("config_file_changed", "event", event.Op.String())

					// Debounce: reset timer if already running
					if debounceTimer != nil {
						debounceTimer.Stop()
					}

					debounceTimer = time.AfterFunc(debounceDuration, func() {
						w.reloadConfig()
					})
				}
			}

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.logger.Error("config_watcher_error", "error", err.Error())
		}
	}
}

// reloadConfig loads the config and calls the onChange callback
func (w *Watcher) reloadConfig() {
	w.logger.Info("reloading_config", "file", w.filepath)

	cfg, err := LoadConfig(w.filepath)
	if err != nil {
		w.logger.Error("config_reload_failed", "error", err.Error())
		return
	}

	if err := w.onChange(cfg); err != nil {
		w.logger.Error("config_apply_failed", "error", err.Error())
		return
	}

	w.logger.Info("config_reloaded_successfully")
}

// isDir checks if path is a directory
func isDir(path string) bool {
	// Simple heuristic: if it has an extension, it's a file
	ext := filepath.Ext(path)
	return ext == ""
}
