package core_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/jdelaire/openslack/core"
	"github.com/jdelaire/openslack/core/ops"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestReloadCommands(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "commands.json")

	// Initial config with two commands.
	os.WriteFile(path, []byte(`[
		{"name":"cmd1","description":"first","command":"echo 1"},
		{"name":"cmd2","description":"second","command":"echo 2"}
	]`), 0644)

	reg := ops.NewRegistry()
	reloader := core.NewReloader(reg, nil, testLogger())

	// Simulate initial load.
	cmds, err := ops.LoadCommands(path)
	if err != nil {
		t.Fatalf("load commands: %v", err)
	}
	var names []string
	for i := range cmds {
		reg.Register(&cmds[i])
		names = append(names, cmds[i].Name())
	}
	reloader.TrackShellOps(names)

	// Verify initial state.
	if reg.Get("cmd1") == nil || reg.Get("cmd2") == nil {
		t.Fatal("expected cmd1 and cmd2 to be registered")
	}

	// Update config: remove cmd2, add cmd3.
	os.WriteFile(path, []byte(`[
		{"name":"cmd1","description":"first-updated","command":"echo 1"},
		{"name":"cmd3","description":"third","command":"echo 3"}
	]`), 0644)

	reloader.ReloadCommands(path)

	// cmd1 should be re-registered, cmd2 gone, cmd3 new.
	if reg.Get("cmd1") == nil {
		t.Error("expected cmd1 after reload")
	}
	if reg.Get("cmd2") != nil {
		t.Error("expected cmd2 to be removed after reload")
	}
	if reg.Get("cmd3") == nil {
		t.Error("expected cmd3 after reload")
	}
}

func TestReloadCommandsInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "commands.json")

	// Start with valid config.
	os.WriteFile(path, []byte(`[{"name":"cmd1","description":"first","command":"echo 1"}]`), 0644)

	reg := ops.NewRegistry()
	reloader := core.NewReloader(reg, nil, testLogger())

	cmds, _ := ops.LoadCommands(path)
	var names []string
	for i := range cmds {
		reg.Register(&cmds[i])
		names = append(names, cmds[i].Name())
	}
	reloader.TrackShellOps(names)

	// Write invalid config.
	os.WriteFile(path, []byte(`invalid json`), 0644)

	reloader.ReloadCommands(path)

	// Old commands should be unregistered (they were removed before parsing).
	// New commands failed to load, so nothing re-registered.
	if reg.Get("cmd1") != nil {
		t.Error("expected cmd1 to be removed even though reload failed")
	}
}

func TestReloadCommandsFileDeleted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "commands.json")

	os.WriteFile(path, []byte(`[{"name":"cmd1","description":"first","command":"echo 1"}]`), 0644)

	reg := ops.NewRegistry()
	reloader := core.NewReloader(reg, nil, testLogger())

	cmds, _ := ops.LoadCommands(path)
	var names []string
	for i := range cmds {
		reg.Register(&cmds[i])
		names = append(names, cmds[i].Name())
	}
	reloader.TrackShellOps(names)

	os.Remove(path)
	reloader.ReloadCommands(path)

	// Old commands unregistered, no new ones loaded (file doesn't exist → nil, nil).
	if reg.Get("cmd1") != nil {
		t.Error("expected cmd1 to be removed after file deleted")
	}
}

func TestReloadPreservesBuiltinOps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "commands.json")

	os.WriteFile(path, []byte(`[{"name":"cmd1","description":"first","command":"echo 1"}]`), 0644)

	reg := ops.NewRegistry()
	reg.Register(&ops.StatusOp{})

	reloader := core.NewReloader(reg, nil, testLogger())

	cmds, _ := ops.LoadCommands(path)
	var names []string
	for i := range cmds {
		reg.Register(&cmds[i])
		names = append(names, cmds[i].Name())
	}
	reloader.TrackShellOps(names)

	// Reload with empty config.
	os.WriteFile(path, []byte(`[]`), 0644)
	reloader.ReloadCommands(path)

	// Built-in status op should still be present.
	if reg.Get("status") == nil {
		t.Error("expected built-in status op to survive reload")
	}
}
