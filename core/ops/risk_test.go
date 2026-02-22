package ops_test

import (
	"testing"

	"github.com/jdelaire/openslack/core/ops"
)

func TestRiskOfDefaultsToLow(t *testing.T) {
	op := &mockOp{name: "test", desc: "no risk classifier"}
	if got := ops.RiskOf(op); got != ops.RiskLow {
		t.Errorf("RiskOf(plain op) = %d, want RiskLow (%d)", got, ops.RiskLow)
	}
}

func TestRiskOfRespectsClassifier(t *testing.T) {
	tests := []struct {
		name string
		op   ops.Op
		want ops.RiskLevel
	}{
		{"RiskNone", &ops.HelpOp{Registry: ops.NewRegistry()}, ops.RiskNone},
		{"RiskLow", &ops.StatusOp{}, ops.RiskLow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ops.RiskOf(tt.op)
			if got != tt.want {
				t.Errorf("RiskOf(%s) = %d, want %d", tt.name, got, tt.want)
			}
		})
	}
}

func TestRiskLevelValues(t *testing.T) {
	if ops.RiskNone >= ops.RiskLow {
		t.Error("RiskNone should be less than RiskLow")
	}
	if ops.RiskLow >= ops.RiskHigh {
		t.Error("RiskLow should be less than RiskHigh")
	}
}

type highRiskOp struct{ mockOp }

func (h *highRiskOp) Risk() ops.RiskLevel { return ops.RiskHigh }

func TestRiskOfHighRisk(t *testing.T) {
	op := &highRiskOp{mockOp{name: "danger", desc: "dangerous"}}
	if got := ops.RiskOf(op); got != ops.RiskHigh {
		t.Errorf("RiskOf(highRisk) = %d, want RiskHigh (%d)", got, ops.RiskHigh)
	}
}
