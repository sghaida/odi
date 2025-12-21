package v3

// Transaction is a tiny domain model used by the v3 example.
type Transaction struct {
	ID          string
	AmountCents int
	Country     string
}

// RiskScore is a computed risk score (0..100-ish).
type RiskScore int

// Decision is the outcome of a fraud/decision flow.
type Decision string

const (
	DecisionApprove      Decision = "APPROVE"
	DecisionManualReview Decision = "MANUAL_REVIEW"
)