package core

import (
	"context"
	"testing"
)

type mockNotifier struct {
	name string
}

func (m *mockNotifier) Name() string                                 { return m.name }
func (m *mockNotifier) Send(_ context.Context, _ Notification) error { return nil }

func TestRegistry_RegisterAndDefault(t *testing.T) {
	r := NewRegistry()
	n := &mockNotifier{name: "telegram"}

	if err := r.Register(n); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	def, err := r.Default()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def.Name() != "telegram" {
		t.Errorf("expected default telegram, got %s", def.Name())
	}
}

func TestRegistry_FirstRegisteredIsDefault(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockNotifier{name: "first"})
	r.Register(&mockNotifier{name: "second"})

	def, err := r.Default()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def.Name() != "first" {
		t.Errorf("expected default first, got %s", def.Name())
	}
}

func TestRegistry_Get(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockNotifier{name: "telegram"})

	n, err := r.Get("telegram")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n.Name() != "telegram" {
		t.Errorf("expected telegram, got %s", n.Name())
	}
}

func TestRegistry_GetNotFound(t *testing.T) {
	r := NewRegistry()
	_, err := r.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing notifier")
	}
}

func TestRegistry_DuplicateRegister(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockNotifier{name: "telegram"})
	err := r.Register(&mockNotifier{name: "telegram"})
	if err == nil {
		t.Fatal("expected error for duplicate registration")
	}
}

func TestRegistry_DefaultEmpty(t *testing.T) {
	r := NewRegistry()
	_, err := r.Default()
	if err == nil {
		t.Fatal("expected error for empty registry")
	}
}
