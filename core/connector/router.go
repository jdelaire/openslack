package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
)

// Router validates and dispatches connector tool calls.
type Router struct {
	cfg     *Config
	manager *Manager
	logger  *slog.Logger
}

// NewRouter creates a tool router.
func NewRouter(cfg *Config, manager *Manager, logger *slog.Logger) *Router {
	return &Router{cfg: cfg, manager: manager, logger: logger}
}

// Call dispatches a connector tool call. The tool name must be in
// "connector.tool" format (e.g., "sample.echo").
func (r *Router) Call(ctx context.Context, qualifiedTool string, args json.RawMessage) (*Response, error) {
	connName, toolName, err := splitTool(qualifiedTool)
	if err != nil {
		return nil, err
	}

	cc, ok := r.cfg.Connectors[connName]
	if !ok {
		return nil, fmt.Errorf("unknown connector %q", connName)
	}

	if !cc.ToolAllowed(toolName) {
		return nil, fmt.Errorf("tool %q not allowed for connector %q", toolName, connName)
	}

	if args == nil {
		args = json.RawMessage(`{}`)
	}

	req := &Request{
		Version: ProtocolVersion,
		ID:      "req_" + uuid.New().String()[:8],
		Tool:    toolName,
		Args:    args,
	}

	r.logger.Info("routing connector call", "connector", connName, "tool", toolName, "id", req.ID)

	resp, err := r.manager.Call(ctx, connName, req)
	if err != nil {
		return nil, fmt.Errorf("connector %q call failed: %w", connName, err)
	}

	return resp, nil
}

// splitTool parses "connector.tool" into its two parts.
func splitTool(qualified string) (connector, tool string, err error) {
	idx := strings.IndexByte(qualified, '.')
	if idx < 0 {
		return "", "", fmt.Errorf("invalid tool name %q: must be connector.tool", qualified)
	}
	connector = qualified[:idx]
	tool = qualified[idx+1:]
	if connector == "" {
		return "", "", fmt.Errorf("invalid tool name %q: empty connector prefix", qualified)
	}
	if tool == "" {
		return "", "", fmt.Errorf("invalid tool name %q: empty tool name", qualified)
	}
	return connector, tool, nil
}
