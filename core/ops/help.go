package ops

import (
	"context"
	"fmt"
	"strings"
)

// HelpOp lists all registered operations.
type HelpOp struct {
	Registry *Registry
}

func (h *HelpOp) Name() string        { return "help" }
func (h *HelpOp) Description() string  { return "List available commands" }
func (h *HelpOp) Risk() RiskLevel      { return RiskNone }

func (h *HelpOp) Execute(_ context.Context, _ string) (string, error) {
	all := h.Registry.List()
	if len(all) == 0 {
		return "No commands available.", nil
	}

	var b strings.Builder
	b.WriteString("Available commands:\n")
	for _, op := range all {
		fmt.Fprintf(&b, "  /%s — %s\n", op.Name(), op.Description())
	}
	return b.String(), nil
}
