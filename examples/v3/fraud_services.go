package v3

import (
	"fmt"

	"github.com/sghaida/odi/examples/v3/config"
)

//go:generate go run ../../cmd/di1 -spec ./specs/fraud.inject.json -out ./fraud_di.gen.go

// Logger is optional; if injected we log.
type Logger interface {
	Infof(format string, args ...any)
}

/*
   Interfaces (what we inject)
*/

// TransactionGetter fetches a transaction from some source (DB/API).
type TransactionGetter interface {
	GetTransaction(id string) (*Transaction, error)
}

// FraudChecker computes a risk score for a transaction.
// Implemented by FraudSvcV3 and injected into DecisionSvcV3 (cycle via interface).
type FraudChecker interface {
	CheckRisk(txID string) (RiskScore, error)
}

// DecisionWriter persists a decision somewhere.
// Implemented by DecisionSvcV3 and injected into FraudSvcV3 (cycle via interface).
type DecisionWriter interface {
	WriteDecision(txID string, decision Decision) error
}

/*
   Concrete service (V3)
*/

// FraudSvc depends on:
//   - TransactionGetter (required)
//   - DecisionWriter (required; creates cycle with DecisionSvcV3)
//   - Logger (optional)
type FraudSvc struct {
	txGetter TransactionGetter
	writer   DecisionWriter
	logger   Logger // optional
}

// NewFraudSvc constructs the service (deps are wired later via generated injectors).
func NewFraudSvc(cfg config.Config) *FraudSvc {
	_ = cfg
	return &FraudSvc{}
}

// CheckRisk is intentionally "pure": it does NOT call writer.
// This prevents recursion even though DecisionSvcV3 calls CheckRisk.
func (f *FraudSvc) CheckRisk(txID string) (RiskScore, error) {
	if f.txGetter == nil {
		return 0, fmt.Errorf("fraud: missing TransactionGetter wiring")
	}

	tx, err := f.txGetter.GetTransaction(txID)
	if err != nil {
		return 0, err
	}

	// tiny fake scoring rule
	score := RiskScore(0)
	if tx.AmountCents > 50_00 {
		score += 30
	}
	if tx.Country == "XX" {
		score += 50
	}

	if f.logger != nil {
		f.logger.Infof("[FraudSvcV3] tx=%s score=%d", tx.ID, score)
	}
	return score, nil
}

// ReviewAndPersist demonstrates FraudSvcV3 using the other side of the cycle (DecisionWriter).
func (f *FraudSvc) ReviewAndPersist(txID string) error {
	if f.writer == nil {
		return fmt.Errorf("fraud: missing DecisionWriter wiring")
	}

	score, err := f.CheckRisk(txID)
	if err != nil {
		return err
	}

	decision := DecisionApprove
	if score >= 60 {
		decision = DecisionManualReview
	}

	if f.logger != nil {
		f.logger.Infof("[FraudSvcV3] writing decision=%s for tx=%s", decision, txID)
	}
	return f.writer.WriteDecision(txID, decision)
}

/*
   Exported setters (what generated Inject(fn) should call)
*/

// SetLogger wires the optional logger.
func (f *FraudSvc) SetLogger(l Logger) { f.logger = l }

// SetTransactionGetter wires the required tx getter.
func (f *FraudSvc) SetTransactionGetter(g TransactionGetter) { f.txGetter = g }

// SetDecisionWriter wires the required decision writer (cycle).
func (f *FraudSvc) SetDecisionWriter(w DecisionWriter) { f.writer = w }
