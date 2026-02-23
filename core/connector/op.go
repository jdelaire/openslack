package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jdelaire/openslack/core/ops"
)

// ConnectorOp bridges a connector tool into the ops.Op interface so it can
// be dispatched from Telegram like any other command.
//
// Telegram usage: /sample.echo hello world
// The args string is passed as {"text": "<args>"} to the connector.
type ConnectorOp struct {
	QualifiedName string // e.g. "sample.echo"
	Desc          string
	Router        *Router
}

func (c *ConnectorOp) Name() string        { return c.QualifiedName }
func (c *ConnectorOp) Description() string  { return c.Desc }
func (c *ConnectorOp) Risk() ops.RiskLevel  { return ops.RiskLow }

func (c *ConnectorOp) Execute(ctx context.Context, args string) (string, error) {
	jsonArgs := argsToJSON(args)

	resp, err := c.Router.Call(ctx, c.QualifiedName, jsonArgs)
	if err != nil {
		return "", err
	}

	if !resp.OK {
		return "", fmt.Errorf("%s: %s", resp.Error.Code, resp.Error.Message)
	}

	return formatData(resp.Data), nil
}

// argsToJSON converts a plain text args string into a JSON object.
// If the string looks like JSON (starts with '{'), it's used as-is.
// Otherwise it's wrapped as {"text": "..."}.
func argsToJSON(args string) json.RawMessage {
	args = strings.TrimSpace(args)
	if args == "" {
		return json.RawMessage(`{}`)
	}
	if strings.HasPrefix(args, "{") {
		return json.RawMessage(args)
	}
	data, _ := json.Marshal(map[string]string{"text": args})
	return data
}

// formatData converts the response data JSON into a readable string.
func formatData(data json.RawMessage) string {
	if data == nil {
		return "OK"
	}

	// Try as a simple string map first for clean output.
	var m map[string]string
	if err := json.Unmarshal(data, &m); err == nil && len(m) > 0 {
		var parts []string
		for k, v := range m {
			parts = append(parts, fmt.Sprintf("%s: %s", k, v))
		}
		return strings.Join(parts, "\n")
	}

	// Fall back to indented JSON.
	pretty, err := json.MarshalIndent(json.RawMessage(data), "", "  ")
	if err != nil {
		return string(data)
	}
	return string(pretty)
}

// RegisterOps creates and registers a ConnectorOp for each allowed tool
// in every configured connector.
func RegisterOps(cfg *Config, router *Router, registry *ops.Registry) error {
	for connName, cc := range cfg.Connectors {
		for _, tool := range cc.Tools {
			qualified := connName + "." + tool
			op := &ConnectorOp{
				QualifiedName: qualified,
				Desc:          fmt.Sprintf("Connector: %s", qualified),
				Router:        router,
			}
			if err := registry.Register(op); err != nil {
				return fmt.Errorf("register connector op %q: %w", qualified, err)
			}
		}
	}
	return nil
}
