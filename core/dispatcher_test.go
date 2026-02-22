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

// --- tests ---

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
