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

// Dispatcher authorizes inbound messages and dispatches commands to ops.
type Dispatcher struct {
	policy   *policy.Policy
	ops      *ops.Registry
	notifier Notifier
	logger   *slog.Logger
	sem      chan struct{}
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

// Handle processes an inbound message: authorize, parse, execute, respond.
func (d *Dispatcher) Handle(msg InboundMessage) {
	if err := d.policy.Authorize(msg.ChatID, msg.UpdateID, msg.Timestamp); err != nil {
		d.logger.Debug("message rejected by policy", "chat_id", msg.ChatID, "error", err)
		return
	}

	cmd, args := parseCommand(msg.Text)
	if cmd == "" {
		return
	}

	op := d.ops.Get(cmd)
	if op == nil {
		d.respond(msg.ChatID, fmt.Sprintf("Unknown command: /%s\nSend /help for available commands.", cmd))
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

	result, err := op.Execute(ctx, args)
	if err != nil {
		d.logger.Error("op failed", "op", cmd, "error", err)
		d.respond(msg.ChatID, fmt.Sprintf("Error running /%s: %s", cmd, err))
		return
	}

	d.respond(msg.ChatID, result)
}

func (d *Dispatcher) respond(chatID int64, text string) {
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
