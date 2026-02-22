package core

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jdelaire/openslack/core/ops"
	"github.com/jdelaire/openslack/core/policy"
)

// --- test helpers ---

type spyNotifier struct {
	mu   sync.Mutex
	sent []Notification
}

func (s *spyNotifier) Name() string { return "spy" }
func (s *spyNotifier) Send(_ context.Context, n Notification) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, n)
	return nil
}
func (s *spyNotifier) lastText() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sent) == 0 {
		return ""
	}
	return s.sent[len(s.sent)-1].Text
}
func (s *spyNotifier) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sent)
}

type echoOp struct{}

func (e *echoOp) Name() string        { return "echo" }
func (e *echoOp) Description() string  { return "echoes args" }
func (e *echoOp) Execute(_ context.Context, args string) (string, error) {
	return "echo: " + args, nil
}

type slowOp struct{}

func (s *slowOp) Name() string        { return "slow" }
func (s *slowOp) Description() string  { return "slow op" }
func (s *slowOp) Execute(ctx context.Context, _ string) (string, error) {
	select {
	case <-time.After(5 * time.Second):
		return "done", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

type errorOp struct{}

func (e *errorOp) Name() string        { return "fail" }
func (e *errorOp) Description() string  { return "always fails" }
func (e *errorOp) Execute(_ context.Context, _ string) (string, error) {
	return "", fmt.Errorf("something broke")
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestDispatcher(spy *spyNotifier, extraOps ...ops.Op) *Dispatcher {
	pol := policy.New([]int64{100})
	reg := ops.NewRegistry()
	for _, op := range extraOps {
		reg.Register(op)
	}
	return NewDispatcher(pol, reg, spy, testLogger())
}

func validMsg(text string) InboundMessage {
	return InboundMessage{
		UpdateID:  time.Now().UnixNano(),
		ChatID:    100,
		UserID:    1,
		Text:      text,
		Timestamp: time.Now(),
	}
}

// --- Phase 3 mock helpers ---

type mockTOTP struct {
	valid bool
}

func (m *mockTOTP) Verify(code string) bool { return m.valid }

type mockLimiter struct {
	locked   bool
	failures int
	resets   int
}

func (m *mockLimiter) Check(_ int64) error {
	if m.locked {
		return fmt.Errorf("rate limited — try again in 15m0s")
	}
	return nil
}
func (m *mockLimiter) RecordFailure(_ int64) { m.failures++ }
func (m *mockLimiter) Reset(_ int64)         { m.resets++ }

type mockApprovals struct {
	nonce     string
	consumed  bool
	createErr error
	opName    string
	opArgs    string
}

func (m *mockApprovals) Create(_ int64, opName, args string) (string, error) {
	if m.createErr != nil {
		return "", m.createErr
	}
	m.opName = opName
	m.opArgs = args
	return m.nonce, nil
}

func (m *mockApprovals) Consume(nonce string, _ int64) (string, string, error) {
	if nonce != m.nonce || m.consumed {
		return "", "", fmt.Errorf("unknown or expired approval nonce")
	}
	m.consumed = true
	return m.opName, m.opArgs, nil
}

// highRiskEchoOp is an echo op that declares itself as RiskHigh.
type highRiskEchoOp struct{}

func (h *highRiskEchoOp) Name() string                                         { return "danger" }
func (h *highRiskEchoOp) Description() string                                  { return "dangerous echo" }
func (h *highRiskEchoOp) Risk() ops.RiskLevel                                  { return ops.RiskHigh }
func (h *highRiskEchoOp) Execute(_ context.Context, args string) (string, error) { return "danger: " + args, nil }

func newSecureDispatcher(spy *spyNotifier, totp *mockTOTP, limiter *mockLimiter, approvals *mockApprovals, extraOps ...ops.Op) *Dispatcher {
	d := newTestDispatcher(spy, extraOps...)
	d.WithSecurity(totp, limiter, approvals)
	return d
}

// --- existing tests (unchanged) ---

func TestDispatchAuthorizedCommand(t *testing.T) {
	spy := &spyNotifier{}
	d := newTestDispatcher(spy, &echoOp{})

	d.Handle(validMsg("/echo hello world"))

	if spy.count() != 1 {
		t.Fatalf("sent %d, want 1", spy.count())
	}
	if got := spy.lastText(); got != "echo: hello world" {
		t.Errorf("text = %q, want %q", got, "echo: hello world")
	}
}

func TestDispatchUnauthorizedChat(t *testing.T) {
	spy := &spyNotifier{}
	d := newTestDispatcher(spy, &echoOp{})

	msg := validMsg("/echo test")
	msg.ChatID = 999

	d.Handle(msg)

	if spy.count() != 0 {
		t.Errorf("sent %d messages for unauthorized chat, want 0", spy.count())
	}
}

func TestDispatchUnknownCommand(t *testing.T) {
	spy := &spyNotifier{}
	d := newTestDispatcher(spy)

	d.Handle(validMsg("/foobar"))

	if spy.count() != 1 {
		t.Fatalf("sent %d, want 1", spy.count())
	}
	if !strings.Contains(spy.lastText(), "Unknown command") {
		t.Errorf("text = %q, want 'Unknown command'", spy.lastText())
	}
	if !strings.Contains(spy.lastText(), "/help") {
		t.Errorf("text = %q, should suggest /help", spy.lastText())
	}
}

func TestDispatchStaleMessage(t *testing.T) {
	spy := &spyNotifier{}
	d := newTestDispatcher(spy, &echoOp{})

	msg := validMsg("/echo test")
	msg.Timestamp = time.Now().Add(-10 * time.Minute)

	d.Handle(msg)

	if spy.count() != 0 {
		t.Errorf("sent %d for stale message, want 0", spy.count())
	}
}

func TestDispatchConcurrencyLimit(t *testing.T) {
	spy := &spyNotifier{}
	d := newTestDispatcher(spy, &slowOp{})

	// Fill the semaphore.
	d.sem <- struct{}{}
	d.sem <- struct{}{}

	d.Handle(validMsg("/slow"))

	// Drain semaphore.
	<-d.sem
	<-d.sem

	if spy.count() != 1 {
		t.Fatalf("sent %d, want 1", spy.count())
	}
	if !strings.Contains(spy.lastText(), "Busy") {
		t.Errorf("text = %q, want 'Busy'", spy.lastText())
	}
}

func TestDispatchOpError(t *testing.T) {
	spy := &spyNotifier{}
	d := newTestDispatcher(spy, &errorOp{})

	d.Handle(validMsg("/fail"))

	if spy.count() != 1 {
		t.Fatalf("sent %d, want 1", spy.count())
	}
	if !strings.Contains(spy.lastText(), "Error running /fail") {
		t.Errorf("text = %q, want error message", spy.lastText())
	}
}

func TestDispatchNonCommand(t *testing.T) {
	spy := &spyNotifier{}
	d := newTestDispatcher(spy, &echoOp{})

	d.Handle(validMsg("just a regular message"))

	if spy.count() != 0 {
		t.Errorf("sent %d for non-command, want 0", spy.count())
	}
}

// --- parseCommand table tests ---

func TestParseCommand(t *testing.T) {
	tests := []struct {
		input   string
		wantCmd string
		wantArgs string
	}{
		{"/status", "status", ""},
		{"/echo hello world", "echo", "hello world"},
		{"/status@mybot", "status", ""},
		{"/echo@mybot hello", "echo", "hello"},
		{"/STATUS", "status", ""},
		{"  /echo  test  ", "echo", "test"},
		{"not a command", "", ""},
		{"", "", ""},
		{"/", "", ""},
	}

	for _, tt := range tests {
		cmd, args := parseCommand(tt.input)
		if cmd != tt.wantCmd || args != tt.wantArgs {
			t.Errorf("parseCommand(%q) = (%q, %q), want (%q, %q)",
				tt.input, cmd, args, tt.wantCmd, tt.wantArgs)
		}
	}
}

// --- Phase 3: extractTOTP tests ---

func TestExtractTOTP(t *testing.T) {
	tests := []struct {
		input    string
		wantArgs string
		wantCode string
	}{
		{"123456", "", "123456"},
		{"hello 123456", "hello", "123456"},
		{"hello world 123456", "hello world", "123456"},
		{"hello", "hello", ""},
		{"", "", ""},
		{"12345", "12345", ""},  // 5 digits - not a code
		{"1234567", "1234567", ""}, // 7 digits - not a code
		{"hello abcdef", "hello abcdef", ""}, // letters not digits
	}

	for _, tt := range tests {
		args, code := extractTOTP(tt.input)
		if args != tt.wantArgs || code != tt.wantCode {
			t.Errorf("extractTOTP(%q) = (%q, %q), want (%q, %q)",
				tt.input, args, code, tt.wantArgs, tt.wantCode)
		}
	}
}

// --- Phase 3: TOTP verification in dispatcher ---

func TestTOTPValidCodeAllowsExecution(t *testing.T) {
	spy := &spyNotifier{}
	totp := &mockTOTP{valid: true}
	limiter := &mockLimiter{}
	d := newSecureDispatcher(spy, totp, limiter, nil, &echoOp{})

	d.Handle(validMsg("/echo hello 123456"))

	if spy.count() != 1 {
		t.Fatalf("sent %d, want 1", spy.count())
	}
	if got := spy.lastText(); got != "echo: hello" {
		t.Errorf("text = %q, want %q", got, "echo: hello")
	}
	if limiter.resets != 1 {
		t.Errorf("resets = %d, want 1", limiter.resets)
	}
}

func TestTOTPInvalidCodeRejects(t *testing.T) {
	spy := &spyNotifier{}
	totp := &mockTOTP{valid: false}
	limiter := &mockLimiter{}
	d := newSecureDispatcher(spy, totp, limiter, nil, &echoOp{})

	d.Handle(validMsg("/echo hello 123456"))

	if spy.count() != 1 {
		t.Fatalf("sent %d, want 1", spy.count())
	}
	if !strings.Contains(spy.lastText(), "Invalid TOTP") {
		t.Errorf("text = %q, want 'Invalid TOTP'", spy.lastText())
	}
	if limiter.failures != 1 {
		t.Errorf("failures = %d, want 1", limiter.failures)
	}
}

func TestTOTPMissingCodeRejects(t *testing.T) {
	spy := &spyNotifier{}
	totp := &mockTOTP{valid: true}
	limiter := &mockLimiter{}
	d := newSecureDispatcher(spy, totp, limiter, nil, &echoOp{})

	d.Handle(validMsg("/echo hello"))

	if spy.count() != 1 {
		t.Fatalf("sent %d, want 1", spy.count())
	}
	if !strings.Contains(spy.lastText(), "requires a TOTP") {
		t.Errorf("text = %q, want TOTP required message", spy.lastText())
	}
}

func TestRiskNoneBypassesToTP(t *testing.T) {
	spy := &spyNotifier{}
	totp := &mockTOTP{valid: false} // TOTP would fail if checked
	limiter := &mockLimiter{}
	helpOp := &ops.HelpOp{Registry: ops.NewRegistry()}
	d := newSecureDispatcher(spy, totp, limiter, nil, helpOp)

	d.Handle(validMsg("/help"))

	if spy.count() != 1 {
		t.Fatalf("sent %d, want 1", spy.count())
	}
	// /help should work even with invalid TOTP mock.
	if strings.Contains(spy.lastText(), "TOTP") {
		t.Errorf("help should not mention TOTP: %q", spy.lastText())
	}
}

func TestRateLimitLocksOutChat(t *testing.T) {
	spy := &spyNotifier{}
	totp := &mockTOTP{valid: true}
	limiter := &mockLimiter{locked: true}
	d := newSecureDispatcher(spy, totp, limiter, nil, &echoOp{})

	d.Handle(validMsg("/echo hello 123456"))

	if spy.count() != 1 {
		t.Fatalf("sent %d, want 1", spy.count())
	}
	if !strings.Contains(spy.lastText(), "Locked out") {
		t.Errorf("text = %q, want lockout message", spy.lastText())
	}
}

func TestHighRiskRejectsDirectExecution(t *testing.T) {
	spy := &spyNotifier{}
	totp := &mockTOTP{valid: true}
	limiter := &mockLimiter{}
	approvals := &mockApprovals{nonce: "abc123"}
	d := newSecureDispatcher(spy, totp, limiter, approvals, &highRiskEchoOp{})

	d.Handle(validMsg("/danger hello 123456"))

	if spy.count() != 1 {
		t.Fatalf("sent %d, want 1", spy.count())
	}
	if !strings.Contains(spy.lastText(), "/do") {
		t.Errorf("text = %q, should suggest /do", spy.lastText())
	}
}

// --- Phase 3: /do and /approve flow ---

func TestDoFlowCreatesPendingApproval(t *testing.T) {
	spy := &spyNotifier{}
	totp := &mockTOTP{valid: true}
	limiter := &mockLimiter{}
	approvals := &mockApprovals{nonce: "abc12345def67890"}
	d := newSecureDispatcher(spy, totp, limiter, approvals, &echoOp{})

	d.Handle(validMsg("/do echo myargs 123456"))

	if spy.count() != 1 {
		t.Fatalf("sent %d, want 1", spy.count())
	}
	if !strings.Contains(spy.lastText(), "abc12345def67890") {
		t.Errorf("text = %q, should contain nonce", spy.lastText())
	}
	if !strings.Contains(spy.lastText(), "/approve") {
		t.Errorf("text = %q, should mention /approve", spy.lastText())
	}
	if approvals.opName != "echo" {
		t.Errorf("opName = %q, want echo", approvals.opName)
	}
	if approvals.opArgs != "myargs" {
		t.Errorf("opArgs = %q, want myargs", approvals.opArgs)
	}
}

func TestDoInvalidTOTP(t *testing.T) {
	spy := &spyNotifier{}
	totp := &mockTOTP{valid: false}
	limiter := &mockLimiter{}
	approvals := &mockApprovals{nonce: "abc123"}
	d := newSecureDispatcher(spy, totp, limiter, approvals, &echoOp{})

	d.Handle(validMsg("/do echo 123456"))

	if !strings.Contains(spy.lastText(), "Invalid TOTP") {
		t.Errorf("text = %q, want 'Invalid TOTP'", spy.lastText())
	}
	if limiter.failures != 1 {
		t.Errorf("failures = %d, want 1", limiter.failures)
	}
}

func TestDoMissingTOTP(t *testing.T) {
	spy := &spyNotifier{}
	totp := &mockTOTP{valid: true}
	limiter := &mockLimiter{}
	approvals := &mockApprovals{nonce: "abc123"}
	d := newSecureDispatcher(spy, totp, limiter, approvals, &echoOp{})

	d.Handle(validMsg("/do echo"))

	if !strings.Contains(spy.lastText(), "requires a TOTP") {
		t.Errorf("text = %q, want TOTP required", spy.lastText())
	}
}

func TestDoUnknownOp(t *testing.T) {
	spy := &spyNotifier{}
	totp := &mockTOTP{valid: true}
	limiter := &mockLimiter{}
	approvals := &mockApprovals{nonce: "abc123"}
	d := newSecureDispatcher(spy, totp, limiter, approvals)

	d.Handle(validMsg("/do nonexistent 123456"))

	if !strings.Contains(spy.lastText(), "Unknown command") {
		t.Errorf("text = %q, want unknown command", spy.lastText())
	}
}

func TestDoEmptyArgs(t *testing.T) {
	spy := &spyNotifier{}
	totp := &mockTOTP{valid: true}
	limiter := &mockLimiter{}
	approvals := &mockApprovals{nonce: "abc123"}
	d := newSecureDispatcher(spy, totp, limiter, approvals, &echoOp{})

	d.Handle(validMsg("/do"))

	if !strings.Contains(spy.lastText(), "Usage") {
		t.Errorf("text = %q, want usage message", spy.lastText())
	}
}

func TestApproveFlowExecutesOp(t *testing.T) {
	spy := &spyNotifier{}
	totp := &mockTOTP{valid: true}
	limiter := &mockLimiter{}
	approvals := &mockApprovals{nonce: "abc12345def67890", opName: "echo", opArgs: "world"}
	d := newSecureDispatcher(spy, totp, limiter, approvals, &echoOp{})

	d.Handle(validMsg("/approve abc12345def67890 123456"))

	if spy.count() != 1 {
		t.Fatalf("sent %d, want 1", spy.count())
	}
	if got := spy.lastText(); got != "echo: world" {
		t.Errorf("text = %q, want %q", got, "echo: world")
	}
	if !approvals.consumed {
		t.Error("approval was not consumed")
	}
}

func TestApproveInvalidTOTP(t *testing.T) {
	spy := &spyNotifier{}
	totp := &mockTOTP{valid: false}
	limiter := &mockLimiter{}
	approvals := &mockApprovals{nonce: "abc123", opName: "echo", opArgs: ""}
	d := newSecureDispatcher(spy, totp, limiter, approvals, &echoOp{})

	d.Handle(validMsg("/approve abc123 123456"))

	if !strings.Contains(spy.lastText(), "Invalid TOTP") {
		t.Errorf("text = %q, want 'Invalid TOTP'", spy.lastText())
	}
}

func TestApproveExpiredNonce(t *testing.T) {
	spy := &spyNotifier{}
	totp := &mockTOTP{valid: true}
	limiter := &mockLimiter{}
	approvals := &mockApprovals{nonce: "abc123", opName: "echo", opArgs: ""}
	approvals.consumed = true // simulate already-consumed
	d := newSecureDispatcher(spy, totp, limiter, approvals, &echoOp{})

	d.Handle(validMsg("/approve abc123 123456"))

	if !strings.Contains(spy.lastText(), "Approval failed") {
		t.Errorf("text = %q, want approval failure", spy.lastText())
	}
}

func TestApproveMissingTOTP(t *testing.T) {
	spy := &spyNotifier{}
	totp := &mockTOTP{valid: true}
	limiter := &mockLimiter{}
	approvals := &mockApprovals{nonce: "abc123", opName: "echo", opArgs: ""}
	d := newSecureDispatcher(spy, totp, limiter, approvals, &echoOp{})

	d.Handle(validMsg("/approve abc123"))

	if !strings.Contains(spy.lastText(), "Usage") {
		t.Errorf("text = %q, want usage", spy.lastText())
	}
}

// --- Phase 3: nil security = no checks (backward compat) ---

func TestNilSecuritySkipsAllChecks(t *testing.T) {
	spy := &spyNotifier{}
	d := newTestDispatcher(spy, &echoOp{})
	// No WithSecurity call — all security is nil.

	d.Handle(validMsg("/echo hello world"))

	if spy.count() != 1 {
		t.Fatalf("sent %d, want 1", spy.count())
	}
	if got := spy.lastText(); got != "echo: hello world" {
		t.Errorf("text = %q, want %q", got, "echo: hello world")
	}
}
