package ratelimit

import (
	"testing"
	"time"
)

func TestCheckAllowsNewChat(t *testing.T) {
	l := New()
	if err := l.Check(100); err != nil {
		t.Errorf("Check(new chat) = %v, want nil", err)
	}
}

func TestCheckAllowsUnderLimit(t *testing.T) {
	l := New()
	for i := 0; i < maxFailures-1; i++ {
		l.RecordFailure(100)
	}
	if err := l.Check(100); err != nil {
		t.Errorf("Check(under limit) = %v, want nil", err)
	}
}

func TestCheckLocksOutAtLimit(t *testing.T) {
	l := New()
	for i := 0; i < maxFailures; i++ {
		l.RecordFailure(100)
	}
	if err := l.Check(100); err == nil {
		t.Error("Check(at limit) = nil, want error")
	}
}

func TestLockoutExpires(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	l := New()
	l.now = func() time.Time { return now }

	for i := 0; i < maxFailures; i++ {
		l.RecordFailure(100)
	}

	// Advance past lockout.
	now = now.Add(lockoutDuration + time.Second)

	if err := l.Check(100); err != nil {
		t.Errorf("Check(after lockout) = %v, want nil", err)
	}
}

func TestOldFailuresExpire(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	l := New()
	l.now = func() time.Time { return now }

	// Record 4 failures.
	for i := 0; i < maxFailures-1; i++ {
		l.RecordFailure(100)
	}

	// Advance past failure window.
	now = now.Add(failureWindow + time.Second)

	// One more failure shouldn't trigger lockout since old ones expired.
	l.RecordFailure(100)
	if err := l.Check(100); err != nil {
		t.Errorf("Check(old failures expired) = %v, want nil", err)
	}
}

func TestIndependentChats(t *testing.T) {
	l := New()
	for i := 0; i < maxFailures; i++ {
		l.RecordFailure(100)
	}
	// Chat 100 is locked, but chat 200 should be fine.
	if err := l.Check(200); err != nil {
		t.Errorf("Check(different chat) = %v, want nil", err)
	}
}

func TestResetClearsState(t *testing.T) {
	l := New()
	for i := 0; i < maxFailures; i++ {
		l.RecordFailure(100)
	}
	l.Reset(100)
	if err := l.Check(100); err != nil {
		t.Errorf("Check(after reset) = %v, want nil", err)
	}
}

func TestRecordFailureAfterReset(t *testing.T) {
	l := New()
	for i := 0; i < maxFailures-1; i++ {
		l.RecordFailure(100)
	}
	l.Reset(100)
	// One failure after reset shouldn't trigger lockout.
	l.RecordFailure(100)
	if err := l.Check(100); err != nil {
		t.Errorf("Check(1 failure after reset) = %v, want nil", err)
	}
}
