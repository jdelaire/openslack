package configwatch

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"time"
)

// Watcher polls files for modification time changes and invokes callbacks.
type Watcher struct {
	interval time.Duration
	logger   *slog.Logger

	mu      sync.Mutex
	entries []watchEntry
}

type watchEntry struct {
	path    string
	modTime time.Time
	cb      func(path string)
}

// New creates a Watcher that polls at the given interval.
func New(interval time.Duration, logger *slog.Logger) *Watcher {
	return &Watcher{
		interval: interval,
		logger:   logger,
	}
}

// Watch adds a file to be watched. The callback is invoked when the file's
// modification time changes. The file does not need to exist at watch time.
func (w *Watcher) Watch(path string, cb func(path string)) {
	w.mu.Lock()
	defer w.mu.Unlock()

	modTime := fileModTime(path)
	w.entries = append(w.entries, watchEntry{
		path:    path,
		modTime: modTime,
		cb:      cb,
	})
}

// Run polls until the context is cancelled. It blocks, so call it in a goroutine.
func (w *Watcher) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.poll()
		}
	}
}

func (w *Watcher) poll() {
	w.mu.Lock()
	defer w.mu.Unlock()

	for i := range w.entries {
		e := &w.entries[i]
		current := fileModTime(e.path)

		// Skip if file doesn't exist (may be mid-save) or unchanged.
		if current.IsZero() || current.Equal(e.modTime) {
			continue
		}

		e.modTime = current
		w.logger.Info("config file changed", "path", e.path)
		e.cb(e.path)
	}
}

// fileModTime returns the file's modification time, or zero if it can't be read.
func fileModTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}
