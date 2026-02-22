package core

import (
	"fmt"
	"sync"
)

// Registry holds named Notifier implementations and tracks the default.
type Registry struct {
	mu          sync.RWMutex
	notifiers   map[string]Notifier
	defaultName string
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		notifiers: make(map[string]Notifier),
	}
}

// Register adds a notifier. The first registered notifier becomes the default.
func (r *Registry) Register(n Notifier) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := n.Name()
	if _, exists := r.notifiers[name]; exists {
		return fmt.Errorf("notifier %q already registered", name)
	}
	r.notifiers[name] = n
	if r.defaultName == "" {
		r.defaultName = name
	}
	return nil
}

// Default returns the default notifier.
func (r *Registry) Default() (Notifier, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.defaultName == "" {
		return nil, fmt.Errorf("no notifiers registered")
	}
	return r.notifiers[r.defaultName], nil
}

// Get returns a notifier by name.
func (r *Registry) Get(name string) (Notifier, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	n, ok := r.notifiers[name]
	if !ok {
		return nil, fmt.Errorf("notifier %q not found", name)
	}
	return n, nil
}
