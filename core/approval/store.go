package approval

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

const (
	nonceBytes = 8
	expiry     = 2 * time.Minute
	maxPending = 100
)

type pending struct {
	chatID    int64
	opName    string
	args      string
	createdAt time.Time
}

// Store holds pending two-step approval requests.
type Store struct {
	mu      sync.Mutex
	items   map[string]*pending
	now     func() time.Time
}

// New creates an approval store.
func New() *Store {
	return &Store{
		items: make(map[string]*pending),
		now:   time.Now,
	}
}

// Create registers a pending operation and returns a nonce.
func (s *Store) Create(chatID int64, opName, args string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pruneLocked()

	if len(s.items) >= maxPending {
		return "", fmt.Errorf("too many pending approvals")
	}

	nonce, err := generateNonce()
	if err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	s.items[nonce] = &pending{
		chatID:    chatID,
		opName:    opName,
		args:      args,
		createdAt: s.now(),
	}
	return nonce, nil
}

// Consume validates and removes a pending approval, returning the op and args.
func (s *Store) Consume(nonce string, chatID int64) (opName, args string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pruneLocked()

	p, ok := s.items[nonce]
	if !ok {
		return "", "", fmt.Errorf("unknown or expired approval nonce")
	}

	if p.chatID != chatID {
		return "", "", fmt.Errorf("approval nonce belongs to a different chat")
	}

	delete(s.items, nonce)
	return p.opName, p.args, nil
}

// pruneLocked removes expired items. Must be called with mu held.
func (s *Store) pruneLocked() {
	now := s.now()
	for nonce, p := range s.items {
		if now.Sub(p.createdAt) > expiry {
			delete(s.items, nonce)
		}
	}
}

func generateNonce() (string, error) {
	b := make([]byte, nonceBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
