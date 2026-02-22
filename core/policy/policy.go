package policy

import (
	"fmt"
	"sync"
	"time"
)

const (
	freshnessWindow = 5 * time.Minute
	maxSeenIDs      = 10000
	pruneCount      = 1000
)

// Policy authorizes inbound messages against a chat allowlist,
// freshness window, and update_id deduplication.
type Policy struct {
	mu       sync.Mutex
	allowed  map[int64]bool
	seen     map[int64]bool
	seenOrder []int64
}

// New creates a Policy that authorizes only the given chat IDs.
func New(chatIDs []int64) *Policy {
	allowed := make(map[int64]bool, len(chatIDs))
	for _, id := range chatIDs {
		allowed[id] = true
	}
	return &Policy{
		allowed:  allowed,
		seen:     make(map[int64]bool),
	}
}

// Authorize checks whether a message should be processed.
func (p *Policy) Authorize(chatID int64, updateID int64, timestamp time.Time) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.allowed[chatID] {
		return fmt.Errorf("unauthorized chat: %d", chatID)
	}

	if time.Since(timestamp) > freshnessWindow {
		return fmt.Errorf("stale message: %v old", time.Since(timestamp).Truncate(time.Second))
	}

	if p.seen[updateID] {
		return fmt.Errorf("duplicate update: %d", updateID)
	}

	// Prune oldest entries if at capacity.
	if len(p.seen) >= maxSeenIDs {
		for i := 0; i < pruneCount && i < len(p.seenOrder); i++ {
			delete(p.seen, p.seenOrder[i])
		}
		p.seenOrder = p.seenOrder[pruneCount:]
	}

	p.seen[updateID] = true
	p.seenOrder = append(p.seenOrder, updateID)

	return nil
}
