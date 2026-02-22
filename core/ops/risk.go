package ops

// RiskLevel classifies how dangerous an operation is.
type RiskLevel int

const (
	RiskNone RiskLevel = iota // No TOTP required (e.g. /help)
	RiskLow                   // TOTP required as last arg
	RiskHigh                  // Two-step /do + /approve flow
)

// RiskClassifier is an optional interface ops may implement to declare
// their risk level. Ops that don't implement it default to RiskLow.
type RiskClassifier interface {
	Risk() RiskLevel
}

// RiskOf returns the risk level of an op. If the op implements
// RiskClassifier, its declared level is used; otherwise RiskLow.
func RiskOf(op Op) RiskLevel {
	if rc, ok := op.(RiskClassifier); ok {
		return rc.Risk()
	}
	return RiskLow
}
