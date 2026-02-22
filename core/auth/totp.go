package auth

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

const (
	totpPeriod = 30 // seconds per time step
	totpDigits = 6
	totpDrift  = 1 // +-1 step tolerance
)

// TOTP implements RFC 6238 time-based one-time passwords with HMAC-SHA1.
type TOTP struct {
	secret []byte
	now    func() time.Time
}

// New creates a TOTP verifier from a base32-encoded secret.
func New(base32Secret string) (*TOTP, error) {
	clean := strings.TrimRight(strings.ToUpper(strings.TrimSpace(base32Secret)), "=")
	// Re-pad to valid base32 length.
	if pad := len(clean) % 8; pad != 0 {
		clean += strings.Repeat("=", 8-pad)
	}
	secret, err := base32.StdEncoding.DecodeString(clean)
	if err != nil {
		return nil, fmt.Errorf("invalid base32 secret: %w", err)
	}
	if len(secret) == 0 {
		return nil, fmt.Errorf("empty secret")
	}
	return &TOTP{secret: secret, now: time.Now}, nil
}

// Verify checks whether code is valid for the current time +-drift.
// Uses constant-time comparison.
func (t *TOTP) Verify(code string) bool {
	if len(code) != totpDigits {
		return false
	}
	counter := t.now().Unix() / totpPeriod
	for offset := -int64(totpDrift); offset <= int64(totpDrift); offset++ {
		expected := generate(t.secret, counter+offset)
		if constantTimeEqual(code, expected) {
			return true
		}
	}
	return false
}

// generate computes a TOTP code for the given counter value.
func generate(secret []byte, counter int64) string {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(counter))

	mac := hmac.New(sha1.New, secret)
	mac.Write(buf)
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	code = code % 1000000

	return fmt.Sprintf("%06d", code)
}

// constantTimeEqual compares two strings in constant time.
func constantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var result byte
	for i := 0; i < len(a); i++ {
		result |= a[i] ^ b[i]
	}
	return result == 0
}
