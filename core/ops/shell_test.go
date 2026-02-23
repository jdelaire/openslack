package ops_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jdelaire/openslack/core/ops"
)

func TestShellOpExecute(t *testing.T) {
	op := &ops.ShellOp{
		CmdName: "echo-test",
		Desc:    "test echo",
		Command: "echo hello",
	}

	if op.Name() != "echo-test" {
		t.Errorf("Name() = %q, want %q", op.Name(), "echo-test")
	}
	if op.Description() != "test echo" {
		t.Errorf("Description() = %q, want %q", op.Description(), "test echo")
	}

	result, err := op.Execute(context.Background(), "")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != "hello" {
		t.Errorf("result = %q, want %q", result, "hello")
	}
}

func TestShellOpWorkDir(t *testing.T) {
	dir := t.TempDir()
	op := &ops.ShellOp{
		CmdName: "pwd-test",
		Desc:    "test workdir",
		Command: "pwd",
		WorkDir: dir,
	}

	result, err := op.Execute(context.Background(), "")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Resolve symlinks on both sides (macOS /tmp -> /private/tmp).
	resolvedDir, _ := filepath.EvalSymlinks(dir)
	resolvedResult, _ := filepath.EvalSymlinks(result)
	if resolvedResult != resolvedDir {
		t.Errorf("result = %q, want %q", resolvedResult, resolvedDir)
	}
}

func TestShellOpWithArgs(t *testing.T) {
	op := &ops.ShellOp{
		CmdName: "greet",
		Desc:    "test args",
		Command: "echo hello",
	}

	result, err := op.Execute(context.Background(), "world")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != "hello world" {
		t.Errorf("result = %q, want %q", result, "hello world")
	}
}

func TestShellOpNoArgs(t *testing.T) {
	op := &ops.ShellOp{
		CmdName: "echo-test",
		Desc:    "test no args",
		Command: "echo hello",
	}

	result, err := op.Execute(context.Background(), "")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != "hello" {
		t.Errorf("result = %q, want %q", result, "hello")
	}
}

func TestShellOpPlaceholder(t *testing.T) {
	op := &ops.ShellOp{
		CmdName: "greet",
		Desc:    "test placeholder",
		Command: `echo "hello {} world"`,
	}

	result, err := op.Execute(context.Background(), "crossfit")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != "hello crossfit world" {
		t.Errorf("result = %q, want %q", result, "hello crossfit world")
	}
}

func TestShellOpPlaceholderEmpty(t *testing.T) {
	op := &ops.ShellOp{
		CmdName: "greet",
		Desc:    "test placeholder with empty args",
		Command: `echo "hello {} world"`,
	}

	result, err := op.Execute(context.Background(), "")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != "hello  world" {
		t.Errorf("result = %q, want %q", result, "hello  world")
	}
}

func TestShellOpFailingCommand(t *testing.T) {
	op := &ops.ShellOp{
		CmdName: "fail-test",
		Desc:    "test failure",
		Command: "exit 1",
	}

	_, err := op.Execute(context.Background(), "")
	if err == nil {
		t.Fatal("expected error from failing command")
	}
}

func TestShellOpDefaultsToRiskLow(t *testing.T) {
	op := &ops.ShellOp{CmdName: "test", Command: "echo hi"}
	if got := ops.RiskOf(op); got != ops.RiskLow {
		t.Errorf("RiskOf(ShellOp) = %d, want RiskLow (%d)", got, ops.RiskLow)
	}
}

func TestLoadCommandsValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "commands.json")
	data := `[{"name":"test-cmd","description":"a test","command":"echo ok","workdir":"/tmp"}]`
	os.WriteFile(path, []byte(data), 0644)

	cmds, err := ops.LoadCommands(path)
	if err != nil {
		t.Fatalf("LoadCommands: %v", err)
	}
	if len(cmds) != 1 {
		t.Fatalf("len = %d, want 1", len(cmds))
	}
	if cmds[0].Name() != "test-cmd" {
		t.Errorf("name = %q, want %q", cmds[0].Name(), "test-cmd")
	}
	if cmds[0].Description() != "a test" {
		t.Errorf("desc = %q, want %q", cmds[0].Description(), "a test")
	}
}

func TestLoadCommandsMissingFile(t *testing.T) {
	cmds, err := ops.LoadCommands("/nonexistent/commands.json")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if cmds != nil {
		t.Fatalf("expected nil commands, got %d", len(cmds))
	}
}

func TestLoadCommandsMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "commands.json")
	os.WriteFile(path, []byte(`not json`), 0644)

	_, err := ops.LoadCommands(path)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "parse commands config") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoadCommandsMissingName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "commands.json")
	os.WriteFile(path, []byte(`[{"command":"echo hi"}]`), 0644)

	_, err := ops.LoadCommands(path)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !strings.Contains(err.Error(), "missing name") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoadCommandsMissingCommand(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "commands.json")
	os.WriteFile(path, []byte(`[{"name":"test"}]`), 0644)

	_, err := ops.LoadCommands(path)
	if err == nil {
		t.Fatal("expected error for missing command")
	}
	if !strings.Contains(err.Error(), "missing command field") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoadCommandsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "commands.json")
	os.WriteFile(path, []byte(`[]`), 0644)

	cmds, err := ops.LoadCommands(path)
	if err != nil {
		t.Fatalf("LoadCommands: %v", err)
	}
	if len(cmds) != 0 {
		t.Fatalf("len = %d, want 0", len(cmds))
	}
}
