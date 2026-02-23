package ops_test

import (
	"context"
	"testing"

	"github.com/jdelaire/openslack/core/ops"
)

type mockOp struct {
	name string
	desc string
}

func (m *mockOp) Name() string                                    { return m.name }
func (m *mockOp) Description() string                             { return m.desc }
func (m *mockOp) Execute(_ context.Context, _ string) (string, error) { return "ok", nil }

func TestRegisterAndGet(t *testing.T) {
	r := ops.NewRegistry()
	op := &mockOp{name: "test", desc: "a test op"}

	if err := r.Register(op); err != nil {
		t.Fatalf("register: %v", err)
	}

	got := r.Get("test")
	if got == nil {
		t.Fatal("expected op, got nil")
	}
	if got.Name() != "test" {
		t.Errorf("name = %q, want %q", got.Name(), "test")
	}
}

func TestGetNotFound(t *testing.T) {
	r := ops.NewRegistry()
	if got := r.Get("missing"); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestDuplicateRegister(t *testing.T) {
	r := ops.NewRegistry()
	op := &mockOp{name: "dup", desc: "first"}
	if err := r.Register(op); err != nil {
		t.Fatalf("first register: %v", err)
	}

	op2 := &mockOp{name: "dup", desc: "second"}
	if err := r.Register(op2); err == nil {
		t.Fatal("expected error on duplicate register")
	}
}

func TestList(t *testing.T) {
	r := ops.NewRegistry()
	r.Register(&mockOp{name: "zebra", desc: "z"})
	r.Register(&mockOp{name: "alpha", desc: "a"})
	r.Register(&mockOp{name: "middle", desc: "m"})

	list := r.List()
	if len(list) != 3 {
		t.Fatalf("len = %d, want 3", len(list))
	}

	expected := []string{"alpha", "middle", "zebra"}
	for i, op := range list {
		if op.Name() != expected[i] {
			t.Errorf("list[%d] = %q, want %q", i, op.Name(), expected[i])
		}
	}
}

func TestUnregister(t *testing.T) {
	r := ops.NewRegistry()
	op := &mockOp{name: "temp", desc: "temporary"}
	r.Register(op)

	if got := r.Get("temp"); got == nil {
		t.Fatal("expected op before unregister")
	}

	r.Unregister("temp")

	if got := r.Get("temp"); got != nil {
		t.Errorf("expected nil after unregister, got %v", got)
	}
}

func TestUnregisterNonExistent(t *testing.T) {
	r := ops.NewRegistry()
	// Should not panic.
	r.Unregister("nonexistent")
}

func TestUnregisterAndReRegister(t *testing.T) {
	r := ops.NewRegistry()
	op1 := &mockOp{name: "cmd", desc: "first"}
	r.Register(op1)

	r.Unregister("cmd")

	op2 := &mockOp{name: "cmd", desc: "second"}
	if err := r.Register(op2); err != nil {
		t.Fatalf("re-register after unregister: %v", err)
	}
	got := r.Get("cmd")
	if got == nil || got.Description() != "second" {
		t.Errorf("expected second op after re-register, got %v", got)
	}
}

func TestListEmpty(t *testing.T) {
	r := ops.NewRegistry()
	list := r.List()
	if len(list) != 0 {
		t.Fatalf("len = %d, want 0", len(list))
	}
}
