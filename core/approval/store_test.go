package approval

import (
	"strings"
	"testing"
	"time"
)

func TestCreateAndConsume(t *testing.T) {
	s := New()
	nonce, err := s.Create(100, "status", "arg1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if nonce == "" {
		t.Fatal("empty nonce")
	}

	opName, args, err := s.Consume(nonce, 100)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if opName != "status" || args != "arg1" {
		t.Errorf("Consume = (%q, %q), want (status, arg1)", opName, args)
	}
}

func TestConsumeDeletesNonce(t *testing.T) {
	s := New()
	nonce, _ := s.Create(100, "status", "")
	s.Consume(nonce, 100)

	_, _, err := s.Consume(nonce, 100)
	if err == nil {
		t.Error("second Consume should fail")
	}
}

func TestConsumeUnknownNonce(t *testing.T) {
	s := New()
	_, _, err := s.Consume("nonexistent", 100)
	if err == nil {
		t.Error("Consume(unknown) should fail")
	}
}

func TestConsumeWrongChat(t *testing.T) {
	s := New()
	nonce, _ := s.Create(100, "status", "")

	_, _, err := s.Consume(nonce, 999)
	if err == nil {
		t.Error("Consume(wrong chat) should fail")
	}
	if !strings.Contains(err.Error(), "different chat") {
		t.Errorf("error = %q, want mention of different chat", err)
	}
}

func TestExpiry(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	s := New()
	s.now = func() time.Time { return now }

	nonce, _ := s.Create(100, "status", "")

	// Advance past expiry.
	now = now.Add(expiry + time.Second)

	_, _, err := s.Consume(nonce, 100)
	if err == nil {
		t.Error("Consume(expired) should fail")
	}
}

func TestCapacityLimit(t *testing.T) {
	s := New()
	for i := 0; i < maxPending; i++ {
		if _, err := s.Create(100, "op", ""); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	_, err := s.Create(100, "op", "")
	if err == nil {
		t.Error("Create beyond capacity should fail")
	}
}

func TestNonceIsHex(t *testing.T) {
	s := New()
	nonce, err := s.Create(100, "test", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// nonceBytes=8 → 16 hex chars.
	if len(nonce) != nonceBytes*2 {
		t.Errorf("nonce length = %d, want %d", len(nonce), nonceBytes*2)
	}
}

func TestExpiryFreesCapacity(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	s := New()
	s.now = func() time.Time { return now }

	for i := 0; i < maxPending; i++ {
		s.Create(100, "op", "")
	}

	// Advance past expiry so prune clears them.
	now = now.Add(expiry + time.Second)

	_, err := s.Create(100, "op", "")
	if err != nil {
		t.Errorf("Create after expiry = %v, want nil", err)
	}
}
