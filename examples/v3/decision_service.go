package v3

import (
	"fmt"

	"github.com/sghaida/odi/examples/v3/config"
)

//go:generate go run ../../cmd/di1 -spec ./specs/decision.inject.json -out ./decision_di.gen.go

// DecisionStore persists decisions (DB, Kafka, HTTP, etc.).
type DecisionStore interface {
	SaveDecision(txID string, decision Decision) error
}

// DecisionSvc depends on:
//   - DecisionStore (required)
//   - FraudChecker (required; cycle with FraudSvc)
//   - Logger (optional)
type DecisionSvc struct {
	store   DecisionStore
	checker FraudChecker
	logger  Logger // optional
}

func NewDecisionSvc(cfg config.Config) *DecisionSvc {
	_ = cfg
	return &DecisionSvc{}
}

// WriteDecision implements DecisionWriter.
// It calls FraudChecker.CheckRisk() for an extra guardrail; CheckRisk is pure so no recursion.
func (d *DecisionSvc) WriteDecision(txID string, decision Decision) error {
	if d.store == nil {
		return fmt.Errorf("decision: missing DecisionStore wiring")
	}
	if d.checker == nil {
		return fmt.Errorf("decision: missing FraudChecker wiring")
	}

	score, err := d.checker.CheckRisk(txID)
	if err != nil {
		return err
	}

	// Example: never allow to approve if score too high.
	if decision == DecisionApprove && score >= 60 {
		decision = DecisionManualReview
	}

	if d.logger != nil {
		d.logger.Infof("[DecisionSvcV3] tx=%s final-decision=%s (score=%d)", txID, decision, score)
	}
	return d.store.SaveDecision(txID, decision)
}

/*
Exported setters (what generated Inject(fn) should call)
*/

func (d *DecisionSvc) SetLogger(l Logger)               { d.logger = l }
func (d *DecisionSvc) SetDecisionStore(s DecisionStore) { d.store = s }
func (d *DecisionSvc) SetFraudChecker(c FraudChecker)   { d.checker = c }
