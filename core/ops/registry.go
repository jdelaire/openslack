package ops

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Op defines an executable operation triggered by an inbound command.
type Op interface {
	Name() string
	Description() string
	Execute(ctx context.Context, args string) (string, error)
}

// Registry holds registered operations keyed by name.
type Registry struct {
	mu  sync.RWMutex
	ops map[string]Op
}

// NewRegistry creates an empty operation registry.
func NewRegistry() *Registry {
	return &Registry{ops: make(map[string]Op)}
}

// Register adds an operation. Returns an error if the name is already registered.
func (r *Registry) Register(op Op) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := op.Name()
	if _, exists := r.ops[name]; exists {
		return fmt.Errorf("op already registered: %s", name)
	}
	r.ops[name] = op
	return nil
}

// Get returns the operation with the given name, or nil if not found.
func (r *Registry) Get(name string) Op {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.ops[name]
}

// List returns all registered operation names sorted alphabetically.
func (r *Registry) List() []Op {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.ops))
	for name := range r.ops {
		names = append(names, name)
	}
	sort.Strings(names)

	result := make([]Op, len(names))
	for i, name := range names {
		result[i] = r.ops[name]
	}
	return result
}
