package ratelimit

import (
	"fmt"
	"sync"
	"time"
)

const (
	maxFailures    = 5
	failureWindow  = 15 * time.Minute
	lockoutDuration = 15 * time.Minute
)

type record struct {
	failures []time.Time
	lockedAt time.Time
}

// Limiter tracks authentication failures per chat ID and locks out
// chats that exceed the failure threshold.
type Limiter struct {
	mu      sync.Mutex
	records map[int64]*record
	now     func() time.Time
}

// New creates a rate limiter.
func New() *Limiter {
	return &Limiter{
		records: make(map[int64]*record),
		now:     time.Now,
	}
}

// Check returns an error if the chat is currently locked out.
func (l *Limiter) Check(chatID int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	r := l.records[chatID]
	if r == nil {
		return nil
	}

	if !r.lockedAt.IsZero() {
		if l.now().Sub(r.lockedAt) < lockoutDuration {
			remaining := lockoutDuration - l.now().Sub(r.lockedAt)
			return fmt.Errorf("rate limited — try again in %s", remaining.Truncate(time.Second))
		}
		// Lockout expired — reset.
		delete(l.records, chatID)
	}
	return nil
}

// RecordFailure records an authentication failure for the given chat.
// If failures exceed the threshold within the window, the chat is locked out.
func (l *Limiter) RecordFailure(chatID int64) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()

	r := l.records[chatID]
	if r == nil {
		r = &record{}
		l.records[chatID] = r
	}

	// Prune old failures outside the window.
	cutoff := now.Add(-failureWindow)
	fresh := r.failures[:0]
	for _, t := range r.failures {
		if t.After(cutoff) {
			fresh = append(fresh, t)
		}
	}
	r.failures = append(fresh, now)

	if len(r.failures) >= maxFailures {
		r.lockedAt = now
	}
}

// Reset clears all failure state for a chat (called on successful auth).
func (l *Limiter) Reset(chatID int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.records, chatID)
}
