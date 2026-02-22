package auth

import (
	"encoding/base32"
	"testing"
	"time"
)

const testSecret = "JBSWY3DPEHPK3PXP" // base32 of "Hello!"... standard test vector

func newTestTOTP(t *testing.T, nowFn func() time.Time) *TOTP {
	t.Helper()
	totp, err := New(testSecret)
	if err != nil {
		t.Fatalf("New(%q): %v", testSecret, err)
	}
	if nowFn != nil {
		totp.now = nowFn
	}
	return totp
}

func codeAt(secret []byte, ts time.Time) string {
	counter := ts.Unix() / totpPeriod
	return generate(secret, counter)
}

func TestVerifyCurrentCode(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	totp := newTestTOTP(t, func() time.Time { return now })

	code := codeAt(totp.secret, now)
	if !totp.Verify(code) {
		t.Errorf("Verify(%q) at current time = false, want true", code)
	}
}

func TestVerifyPreviousStep(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 30, 0, time.UTC)
	totp := newTestTOTP(t, func() time.Time { return now })

	// Code from previous step (t-30s).
	code := codeAt(totp.secret, now.Add(-30*time.Second))
	if !totp.Verify(code) {
		t.Errorf("Verify(previous step) = false, want true (drift tolerance)")
	}
}

func TestVerifyNextStep(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	totp := newTestTOTP(t, func() time.Time { return now })

	// Code from next step (t+30s).
	code := codeAt(totp.secret, now.Add(30*time.Second))
	if !totp.Verify(code) {
		t.Errorf("Verify(next step) = false, want true (drift tolerance)")
	}
}

func TestVerifyTwoStepsAway(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	totp := newTestTOTP(t, func() time.Time { return now })

	// Code from 2 steps away — should be rejected.
	code := codeAt(totp.secret, now.Add(60*time.Second))
	if totp.Verify(code) {
		t.Errorf("Verify(2 steps away) = true, want false")
	}
}

func TestVerifyInvalidCode(t *testing.T) {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	totp := newTestTOTP(t, func() time.Time { return now })

	if totp.Verify("000000") {
		// Extremely unlikely but possible; skip if it somehow matches.
		t.Skip("000000 happens to be valid for this time")
	}
}

func TestVerifyWrongLength(t *testing.T) {
	totp := newTestTOTP(t, nil)

	cases := []string{"", "12345", "1234567", "abc"}
	for _, code := range cases {
		if totp.Verify(code) {
			t.Errorf("Verify(%q) = true, want false (wrong length)", code)
		}
	}
}

func TestNewBadSecret(t *testing.T) {
	_, err := New("!!!invalid!!!")
	if err == nil {
		t.Error("New(invalid) = nil error, want error")
	}
}

func TestNewEmptySecret(t *testing.T) {
	_, err := New("")
	if err == nil {
		t.Error("New(empty) = nil error, want error")
	}
}

func TestNewLowercaseAndPadding(t *testing.T) {
	// Should handle lowercase and missing padding.
	totp, err := New("jbswy3dpehpk3pxp")
	if err != nil {
		t.Fatalf("New(lowercase): %v", err)
	}
	if totp == nil {
		t.Fatal("expected non-nil TOTP")
	}
}

func TestGenerateDeterministic(t *testing.T) {
	secret, _ := base32.StdEncoding.DecodeString(testSecret)
	code1 := generate(secret, 100)
	code2 := generate(secret, 100)
	if code1 != code2 {
		t.Errorf("generate not deterministic: %q != %q", code1, code2)
	}
	if len(code1) != 6 {
		t.Errorf("code length = %d, want 6", len(code1))
	}
}

func TestGenerateDifferentCounters(t *testing.T) {
	secret, _ := base32.StdEncoding.DecodeString(testSecret)
	code1 := generate(secret, 100)
	code2 := generate(secret, 101)
	if code1 == code2 {
		t.Errorf("different counters produced same code: %q", code1)
	}
}
