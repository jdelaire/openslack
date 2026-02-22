package core

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jdelaire/openslack/core/ops"
	"github.com/jdelaire/openslack/core/policy"
)

const (
	maxConcurrentOps = 2
	opTimeout        = 30 * time.Second
)

// TOTPVerifier verifies time-based one-time passwords.
type TOTPVerifier interface {
	Verify(code string) bool
}

// RateLimiter tracks authentication failures and enforces lockouts.
type RateLimiter interface {
	Check(chatID int64) error
	RecordFailure(chatID int64)
	Reset(chatID int64)
}

// ApprovalStore manages pending two-step approval requests.
type ApprovalStore interface {
	Create(chatID int64, opName, args string) (nonce string, err error)
	Consume(nonce string, chatID int64) (opName, args string, err error)
}

// Dispatcher authorizes inbound messages and dispatches commands to ops.
type Dispatcher struct {
	policy    *policy.Policy
	ops       *ops.Registry
	notifier  Notifier
	logger    *slog.Logger
	sem       chan struct{}
	totp      TOTPVerifier
	limiter   RateLimiter
	approvals ApprovalStore
}

// NewDispatcher creates a Dispatcher.
func NewDispatcher(pol *policy.Policy, opsReg *ops.Registry, notifier Notifier, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{
		policy:   pol,
		ops:      opsReg,
		notifier: notifier,
		logger:   logger,
		sem:      make(chan struct{}, maxConcurrentOps),
	}
}

// WithSecurity attaches Phase 3 security components. Nil values disable
// the corresponding check, so existing callers are unaffected.
func (d *Dispatcher) WithSecurity(totp TOTPVerifier, limiter RateLimiter, approvals ApprovalStore) *Dispatcher {
	d.totp = totp
	d.limiter = limiter
	d.approvals = approvals
	return d
}

// Handle processes an inbound message: authorize, parse, execute, respond.
func (d *Dispatcher) Handle(msg InboundMessage) {
	if err := d.policy.Authorize(msg.ChatID, msg.UpdateID, msg.Timestamp); err != nil {
		d.logger.Debug("message rejected by policy", "chat_id", msg.ChatID, "error", err)
		return
	}

	// Rate limit check.
	if d.limiter != nil {
		if err := d.limiter.Check(msg.ChatID); err != nil {
			d.respond(msg.ChatID, fmt.Sprintf("Locked out: %s", err))
			return
		}
	}

	cmd, args := parseCommand(msg.Text)
	if cmd == "" {
		return
	}

	// Built-in two-step commands.
	if cmd == "do" && d.approvals != nil && d.totp != nil {
		d.logger.Info("command received", "cmd", cmd, "chat_id", msg.ChatID)
		d.handleDo(msg, args)
		return
	}
	if cmd == "approve" && d.approvals != nil && d.totp != nil {
		d.logger.Info("command received", "cmd", cmd, "chat_id", msg.ChatID)
		d.handleApprove(msg, args)
		return
	}

	d.logger.Info("command received", "cmd", cmd, "chat_id", msg.ChatID)

	op := d.ops.Get(cmd)
	if op == nil {
		d.respond(msg.ChatID, fmt.Sprintf("Unknown command: /%s\nSend /help for available commands.", cmd))
		return
	}

	risk := ops.RiskOf(op)

	// Risk-level branching.
	switch risk {
	case ops.RiskNone:
		// Execute immediately, no TOTP.
	case ops.RiskLow:
		if d.totp != nil {
			realArgs, code := extractTOTP(args)
			if code == "" {
				d.recordFailure(msg.ChatID)
				d.respond(msg.ChatID, fmt.Sprintf("/%s requires a TOTP code as the last argument.", cmd))
				return
			}
			if !d.totp.Verify(code) {
				d.recordFailure(msg.ChatID)
				d.respond(msg.ChatID, "Invalid TOTP code.")
				return
			}
			d.resetFailures(msg.ChatID)
			args = realArgs
		}
	case ops.RiskHigh:
		if d.totp != nil {
			d.respond(msg.ChatID, fmt.Sprintf("/%s is a high-risk operation. Use /do %s <args> <totp> for two-step approval.", cmd, cmd))
			return
		}
	}

	// Non-blocking semaphore acquire.
	select {
	case d.sem <- struct{}{}:
	default:
		d.respond(msg.ChatID, "Busy — too many operations running. Try again shortly.")
		return
	}

	defer func() { <-d.sem }()

	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	result, err := op.Execute(ctx, args)
	if err != nil {
		d.logger.Error("op failed", "op", cmd, "error", err)
		d.respond(msg.ChatID, fmt.Sprintf("Error running /%s: %s", cmd, err))
		return
	}

	d.logger.Info("command completed", "cmd", cmd, "chat_id", msg.ChatID)
	d.respond(msg.ChatID, result)
}

// handleDo initiates a two-step approval: /do <opName> [args] <totp>
func (d *Dispatcher) handleDo(msg InboundMessage, args string) {
	parts := strings.SplitN(args, " ", 2)
	if len(parts) == 0 || parts[0] == "" {
		d.respond(msg.ChatID, "Usage: /do <command> [args] <totp>")
		return
	}

	opName := parts[0]
	opArgs := ""
	if len(parts) > 1 {
		opArgs = parts[1]
	}

	// Extract TOTP from the end of opArgs.
	realArgs, code := extractTOTP(opArgs)
	if code == "" {
		d.recordFailure(msg.ChatID)
		d.respond(msg.ChatID, "/do requires a TOTP code as the last argument.")
		return
	}

	if !d.totp.Verify(code) {
		d.recordFailure(msg.ChatID)
		d.respond(msg.ChatID, "Invalid TOTP code.")
		return
	}
	d.resetFailures(msg.ChatID)

	// Verify op exists.
	op := d.ops.Get(opName)
	if op == nil {
		d.respond(msg.ChatID, fmt.Sprintf("Unknown command: /%s", opName))
		return
	}

	nonce, err := d.approvals.Create(msg.ChatID, opName, realArgs)
	if err != nil {
		d.respond(msg.ChatID, fmt.Sprintf("Failed to create approval: %s", err))
		return
	}

	d.respond(msg.ChatID, fmt.Sprintf("Pending approval for /%s. Send:\n/approve %s <totp>", opName, nonce))
}

// handleApprove completes a two-step approval: /approve <nonce> <totp>
func (d *Dispatcher) handleApprove(msg InboundMessage, args string) {
	realArgs, code := extractTOTP(args)
	if code == "" {
		d.recordFailure(msg.ChatID)
		d.respond(msg.ChatID, "Usage: /approve <nonce> <totp>")
		return
	}

	if !d.totp.Verify(code) {
		d.recordFailure(msg.ChatID)
		d.respond(msg.ChatID, "Invalid TOTP code.")
		return
	}
	d.resetFailures(msg.ChatID)

	nonce := strings.TrimSpace(realArgs)
	opName, opArgs, err := d.approvals.Consume(nonce, msg.ChatID)
	if err != nil {
		d.respond(msg.ChatID, fmt.Sprintf("Approval failed: %s", err))
		return
	}

	op := d.ops.Get(opName)
	if op == nil {
		d.respond(msg.ChatID, fmt.Sprintf("Operation /%s no longer registered.", opName))
		return
	}

	// Non-blocking semaphore acquire.
	select {
	case d.sem <- struct{}{}:
	default:
		d.respond(msg.ChatID, "Busy — too many operations running. Try again shortly.")
		return
	}
	defer func() { <-d.sem }()

	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	result, err := op.Execute(ctx, opArgs)
	if err != nil {
		d.logger.Error("op failed", "op", opName, "error", err)
		d.respond(msg.ChatID, fmt.Sprintf("Error running /%s: %s", opName, err))
		return
	}

	d.logger.Info("command completed", "cmd", opName, "chat_id", msg.ChatID)
	d.respond(msg.ChatID, result)
}

func (d *Dispatcher) recordFailure(chatID int64) {
	if d.limiter != nil {
		d.limiter.RecordFailure(chatID)
	}
}

func (d *Dispatcher) resetFailures(chatID int64) {
	if d.limiter != nil {
		d.limiter.Reset(chatID)
	}
}

const maxMessageLen = 4096

func (d *Dispatcher) respond(chatID int64, text string) {
	if len(text) > maxMessageLen {
		text = "…" + text[len(text)-maxMessageLen+len("…"):]
	}
	n := Notification{
		Text:      text,
		Source:    "dispatcher",
		CreatedAt: time.Now(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := d.notifier.Send(ctx, n); err != nil {
		d.logger.Error("failed to send response", "chat_id", chatID, "error", err)
	}
}

// extractTOTP splits a 6-digit TOTP code from the last token of args.
// Returns (remainingArgs, code). If no valid code found, code is "".
func extractTOTP(args string) (realArgs, code string) {
	args = strings.TrimSpace(args)
	if args == "" {
		return "", ""
	}

	lastSpace := strings.LastIndex(args, " ")
	if lastSpace == -1 {
		// The entire args string might be the code.
		if isTOTPCode(args) {
			return "", args
		}
		return args, ""
	}

	candidate := args[lastSpace+1:]
	if isTOTPCode(candidate) {
		return strings.TrimSpace(args[:lastSpace]), candidate
	}
	return args, ""
}

func isTOTPCode(s string) bool {
	if len(s) != 6 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// parseCommand extracts the command name and arguments from a message.
// It handles "/command", "/command args", and "/command@botname args".
func parseCommand(text string) (cmd, args string) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return "", ""
	}

	text = text[1:] // strip leading "/"
	parts := strings.SplitN(text, " ", 2)
	cmd = parts[0]
	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}

	// Strip @botname suffix.
	if at := strings.Index(cmd, "@"); at != -1 {
		cmd = cmd[:at]
	}

	cmd = strings.ToLower(cmd)
	return cmd, args
}
