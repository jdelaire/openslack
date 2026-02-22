package policy_test

import (
	"strings"
	"testing"
	"time"

	"github.com/jdelaire/openslack/core/policy"
)

func TestAuthorizeAllowedChat(t *testing.T) {
	p := policy.New([]int64{100, 200})
	err := p.Authorize(100, 1, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthorizeDeniedChat(t *testing.T) {
	p := policy.New([]int64{100})
	err := p.Authorize(999, 1, time.Now())
	if err == nil {
		t.Fatal("expected error for unauthorized chat")
	}
	if !strings.Contains(err.Error(), "unauthorized chat") {
		t.Errorf("error = %q, want 'unauthorized chat'", err)
	}
}

func TestAuthorizeStaleMessage(t *testing.T) {
	p := policy.New([]int64{100})
	stale := time.Now().Add(-6 * time.Minute)
	err := p.Authorize(100, 1, stale)
	if err == nil {
		t.Fatal("expected error for stale message")
	}
	if !strings.Contains(err.Error(), "stale message") {
		t.Errorf("error = %q, want 'stale message'", err)
	}
}

func TestAuthorizeDuplicateUpdateID(t *testing.T) {
	p := policy.New([]int64{100})
	now := time.Now()

	if err := p.Authorize(100, 42, now); err != nil {
		t.Fatalf("first: %v", err)
	}

	err := p.Authorize(100, 42, now)
	if err == nil {
		t.Fatal("expected error for duplicate update_id")
	}
	if !strings.Contains(err.Error(), "duplicate update") {
		t.Errorf("error = %q, want 'duplicate update'", err)
	}
}

func TestAuthorizePruning(t *testing.T) {
	p := policy.New([]int64{100})
	now := time.Now()

	// Fill up to capacity.
	for i := int64(0); i < 10000; i++ {
		if err := p.Authorize(100, i, now); err != nil {
			t.Fatalf("authorize %d: %v", i, err)
		}
	}

	// Next authorize should trigger pruning and succeed.
	if err := p.Authorize(100, 10000, now); err != nil {
		t.Fatalf("post-prune authorize: %v", err)
	}

	// Early IDs should be pruned and reusable.
	if err := p.Authorize(100, 0, now); err != nil {
		t.Fatalf("reuse pruned ID: %v", err)
	}
}

func TestAuthorizeEmptyAllowlist(t *testing.T) {
	p := policy.New(nil)
	err := p.Authorize(100, 1, time.Now())
	if err == nil {
		t.Fatal("expected error for empty allowlist")
	}
}
