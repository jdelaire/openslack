package configwatch_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jdelaire/openslack/core/configwatch"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestWatcherDetectsChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"v":1}`), 0644)

	var called atomic.Int32
	w := configwatch.New(50*time.Millisecond, testLogger())
	w.Watch(path, func(_ string) {
		called.Add(1)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	// Wait for at least one poll cycle, then modify the file.
	time.Sleep(100 * time.Millisecond)
	os.WriteFile(path, []byte(`{"v":2}`), 0644)

	// Wait for callback to fire.
	deadline := time.After(2 * time.Second)
	for called.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for change callback")
		default:
			time.Sleep(20 * time.Millisecond)
		}
	}
}

func TestWatcherNoCallbackWithoutChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"v":1}`), 0644)

	var called atomic.Int32
	w := configwatch.New(50*time.Millisecond, testLogger())
	w.Watch(path, func(_ string) {
		called.Add(1)
	})

	ctx, cancel := context.WithCancel(context.Background())
	go w.Run(ctx)

	time.Sleep(200 * time.Millisecond)
	cancel()

	if called.Load() != 0 {
		t.Errorf("callback fired %d times without file change", called.Load())
	}
}

func TestWatcherHandlesDeletedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"v":1}`), 0644)

	var called atomic.Int32
	w := configwatch.New(50*time.Millisecond, testLogger())
	w.Watch(path, func(_ string) {
		called.Add(1)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	time.Sleep(100 * time.Millisecond)
	os.Remove(path)

	// Should not panic or fire callback for deletion.
	time.Sleep(200 * time.Millisecond)
	if called.Load() != 0 {
		t.Errorf("callback fired %d times for deleted file", called.Load())
	}
}

func TestWatcherHandlesNonExistentFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.json")

	var called atomic.Int32
	w := configwatch.New(50*time.Millisecond, testLogger())
	w.Watch(path, func(_ string) {
		called.Add(1)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	// File appears after watch starts.
	time.Sleep(100 * time.Millisecond)
	os.WriteFile(path, []byte(`{"v":1}`), 0644)

	deadline := time.After(2 * time.Second)
	for called.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for callback on new file")
		default:
			time.Sleep(20 * time.Millisecond)
		}
	}
}

func TestWatcherStopsOnContextCancel(t *testing.T) {
	w := configwatch.New(50*time.Millisecond, testLogger())

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// Run exited cleanly.
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after context cancel")
	}
}
