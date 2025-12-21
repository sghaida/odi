// odi/examples/v3/main/main.go
package main

import (
	"fmt"
	"os"

	v3 "github.com/sghaida/odi/examples/v3"
	"github.com/sghaida/odi/examples/v3/config"
)

/*
This main is intentionally "documentation in code".

It demonstrates di1 usage end-to-end:
- when to use each step
- why it exists
- how to use it correctly (including the required-deps "hasX" behavior)
- how to wire a safe cycle via interfaces

Key idea:
di1 generates a *facade/builder* around a concrete service.
You do NOT call your service constructor directly in main.
You call New<Facade>(cfg), then Inject deps, then Build.
*/

type stdLogger struct{}

func (l stdLogger) Infof(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stdout, format+"\n", args...)
}

/*
STEP: Provide concrete implementations for the injected interfaces
WHEN to do this:
- At the composition root (main/server startup) and in tests.
WHY:
- Services depend on interfaces (TransactionGetter, DecisionStore, etc.).
- Concrete implementations can be swapped (DB, HTTP, mocks).
HOW:
- Implement the interface methods and pass them to Inject<Name>(dep).
*/

// implements v3.TransactionGetter
type inMemoryTxRepo struct {
	data map[string]*v3.Transaction
}

func (r inMemoryTxRepo) GetTransaction(id string) (*v3.Transaction, error) {
	tx, ok := r.data[id]
	if !ok {
		return nil, fmt.Errorf("tx not found: %s", id)
	}
	return tx, nil
}

// implements v3.DecisionStore
type inMemoryDecisionStore struct {
	data map[string]v3.Decision
}

func (s *inMemoryDecisionStore) SaveDecision(txID string, decision v3.Decision) error {
	if s.data == nil {
		s.data = make(map[string]v3.Decision)
	}
	s.data[txID] = decision
	return nil
}

func main() {
	/*
		STEP: Create config
		WHEN:
		- Always. di1-generated facades take config.Config as input.
		WHY:
		- Keeps construction consistent and allows services to read runtime config.
		HOW:
		- Fill minimal fields used by your example.
	*/
	cfg := config.Config{Env: "dev"}

	/*
		STEP: Create di1-generated facades/builders
		WHEN:
		- Always at the composition root (main/startup).
		WHY:
		- di1 facade enforces explicit injection and validates required deps at build time.
		HOW:
		- Call New<FacadeName>(cfg), NOT the concrete constructor.
		  FacadeName comes from wrapperBase + versionSuffix in your *.inject.json.
	*/
	fraudB := v3.NewFraudSvcV3(cfg)
	decisionB := v3.NewDecisionSvcV3(cfg)

	/*
		STEP: Prepare concrete deps (implementations)
		WHEN:
		- Before injection.
		WHY:
		- Required deps must be available to wire before Build().
		HOW:
		- Construct any required dependencies here (repos, stores, clients, etc).
	*/
	txRepo := inMemoryTxRepo{
		data: map[string]*v3.Transaction{
			"tx-1": {ID: "tx-1", AmountCents: 12_00, Country: "DE"},
			"tx-2": {ID: "tx-2", AmountCents: 90_00, Country: "XX"},
		},
	}
	store := &inMemoryDecisionStore{}
	log := stdLogger{}

	/*
		STEP: Inject REQUIRED dependencies using Inject<Name>
		WHEN:
		- Always for required deps listed in *.inject.json under "required".
		WHY:
		- Inject<Name> does two things:
		  1) sets the field on the concrete service
		  2) flips a facade boolean flag (hasX=true) so Build() can validate wiring

		  If you set required deps via setters in Inject(fn) only,
		  Build() will still fail with "missing required dep ...".
		HOW:
		- Call the generated Inject<Name>(dep) methods.
	*/
	fraudB.InjectTransactionGetter(txRepo)
	decisionB.InjectDecisionStore(store)

	/*
		STEP: Inject OPTIONAL dependencies (and custom wiring) using Inject(fn)
		WHEN:
		- Optional deps (like Logger), or any extra custom wiring not represented as a required dep.
		WHY:
		- Optional deps should not block Build().
		- Inject(fn) gives you access to the underlying concrete service pointer.
		HOW:
		- Call exported setters from your service (SetLogger, etc).
	*/
	fraudB.Inject(func(s *v3.FraudSvc) { s.SetLogger(log) })
	decisionB.Inject(func(s *v3.DecisionSvc) { s.SetLogger(log) })

	/*
		STEP: Wire a cycle safely (FraudSvc <-> DecisionSvc) using interfaces

		WHEN:
		- Two services need to call each other, but you want to avoid hard coupling and recursion.
		WHY:
		- di1 can wire cycles as long as you do it through interfaces and keep recursion in check.
		- In this example:
		  - FraudSvc requires DecisionWriter (implemented by DecisionSvc)
		  - DecisionSvc requires FraudChecker (implemented by FraudSvc)
		HOW:
		- Capture the *concrete service pointers* held inside each builder.
		- Then inject the cycle endpoints using Inject<Name> (required deps!)
	*/
	var fraudSvc *v3.FraudSvc
	var decisionSvc *v3.DecisionSvc

	// Capture pointers: builders already constructed the services internally.
	fraudB.Inject(func(s *v3.FraudSvc) { fraudSvc = s })
	decisionB.Inject(func(s *v3.DecisionSvc) { decisionSvc = s })

	/*
		CRITICAL: Use Inject<Name> for REQUIRED cycle deps
		WHY:
		- Inject<Name> marks hasX=true so Build() succeeds.
	*/
	fraudB.InjectDecisionWriter(decisionSvc) // DecisionSvc implements DecisionWriter
	decisionB.InjectFraudChecker(fraudSvc)   // FraudSvc implements FraudChecker

	/*
		STEP: Build services
		WHEN:
		- After all required deps are injected.
		WHY:
		- Build() validates you didn't forget required wiring (fast failure).
		HOW:
		- Use Build() to handle errors or MustBuild() to panic.
	*/
	fraudSvcFinal, err := fraudB.Build()
	if err != nil {
		panic(err)
	}
	_ = decisionB.MustBuild()

	/*
		STEP: Run business logic
		WHEN:
		- After successful build.
		WHY:
		- Services are now fully wired and safe to use.
		HOW:
		- Call methods on the concrete services returned by Build/MustBuild.
	*/
	fmt.Println("---- ReviewAndPersist(tx-1) ----")
	if err := fraudSvcFinal.ReviewAndPersist("tx-1"); err != nil {
		panic(err)
	}

	fmt.Println("---- ReviewAndPersist(tx-2) ----")
	if err := fraudSvcFinal.ReviewAndPersist("tx-2"); err != nil {
		panic(err)
	}

	fmt.Println("---- Stored decisions ----")
	for txID, d := range store.data {
		fmt.Printf("%s => %s\n", txID, d)
	}

	/*
		Quick debugging checklist (common issues):
		- "Unresolved reference InjectX/Build/MustBuild":
		  => you're using the concrete constructor (NewDecisionSvc) instead of NewDecisionSvcV3 facade.
		- "not wired: missing required dep X":
		  => you set X via setters in Inject(fn) but did not call InjectX(dep) to flip hasX=true.
		- cycles:
		  => ensure the call graph is not recursive (e.g., keep CheckRisk pure as you did).
	*/
}
