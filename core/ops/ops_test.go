package ops_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jdelaire/openslack/core/ops"
)

func TestStatusOutput(t *testing.T) {
	op := &ops.StatusOp{}
	result, err := op.Execute(context.Background(), "")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(result, "Status: OK") {
		t.Errorf("missing 'Status: OK' in %q", result)
	}
	if !strings.Contains(result, "Uptime:") {
		t.Errorf("missing 'Uptime:' in %q", result)
	}
	if !strings.Contains(result, "Go:") {
		t.Errorf("missing 'Go:' in %q", result)
	}
	if !strings.Contains(result, "Goroutines:") {
		t.Errorf("missing 'Goroutines:' in %q", result)
	}
}

func TestStatusName(t *testing.T) {
	op := &ops.StatusOp{}
	if op.Name() != "status" {
		t.Errorf("name = %q, want 'status'", op.Name())
	}
}

func TestHelpOutput(t *testing.T) {
	reg := ops.NewRegistry()
	reg.Register(&ops.StatusOp{})
	reg.Register(&ops.HelpOp{Registry: reg})

	op := &ops.HelpOp{Registry: reg}
	result, err := op.Execute(context.Background(), "")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	if !strings.Contains(result, "/help") {
		t.Errorf("missing /help in output: %q", result)
	}
	if !strings.Contains(result, "/status") {
		t.Errorf("missing /status in output: %q", result)
	}
	if !strings.Contains(result, "Available commands:") {
		t.Errorf("missing header in output: %q", result)
	}
}

func TestHelpSortOrder(t *testing.T) {
	reg := ops.NewRegistry()
	reg.Register(&mockOp{name: "zebra", desc: "z"})
	reg.Register(&mockOp{name: "alpha", desc: "a"})

	op := &ops.HelpOp{Registry: reg}
	result, err := op.Execute(context.Background(), "")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	alphaIdx := strings.Index(result, "/alpha")
	zebraIdx := strings.Index(result, "/zebra")
	if alphaIdx == -1 || zebraIdx == -1 {
		t.Fatalf("missing commands in output: %q", result)
	}
	if alphaIdx > zebraIdx {
		t.Errorf("alpha should come before zebra in sorted output")
	}
}

func TestHelpEmpty(t *testing.T) {
	reg := ops.NewRegistry()
	op := &ops.HelpOp{Registry: reg}
	result, err := op.Execute(context.Background(), "")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(result, "No commands available") {
		t.Errorf("expected empty message, got: %q", result)
	}
}
