package v4

import (
	"context"
	"fmt"
	"time"

	"github.com/sghaida/odi/examples/v4/config"
)

// Core depends on Alpha + Beta (required) and optionally uses Tracer + Metrics.
//
// Required deps are injected via generated facade methods:
//
//	coreB.InjectAlpha(alphaB.UnsafeImpl())
//	coreB.InjectBeta(betaB.UnsafeImpl())
//
// Optional deps are applied via BuildWith(reg) using registry keys from spec.

//go:generate go run ../../cmd/di2 -spec specs/core.inject.json -out core_v4.gen.go

type Core struct {
	cfg config.Config

	// Required deps (must be injected before Build()):
	alpha *Alpha
	beta  *Beta

	// Optional deps (applied during BuildWith(reg) if available):
	tracer  Tracer
	metrics Metrics
}

// NewCore is the constructor used by the generated facade (CoreV4).
func NewCore(cfg config.Config) *Core {
	fmt.Println("[core] NewCore: constructor called")

	return &Core{
		cfg:     cfg,
		tracer:  NoopTracer{},
		metrics: NoopMetrics{},
	}
}

// SetTracer demonstrates optional dep "setter" apply (OptionalApply.Kind = "setter").
func (c *Core) SetTracer(t Tracer) {
	fmt.Printf("[core] optional dep applied: Tracer (%T)\n", t)
	c.tracer = t
}

// SetMetrics is an optional-dep setter (use apply.kind="setter" in spec).
func (c *Core) SetMetrics(m Metrics) {
	if m == nil {
		fmt.Println("[core] optional dep missing: Metrics → using NoopMetrics")
		c.metrics = NoopMetrics{}
		return
	}
	fmt.Printf("[core] optional dep applied: Metrics (%T)\n", m)
	c.metrics = m
}

// Process is Core's main business method.
// The generated facade will enforce required wiring at runtime before calling this,
// so calling Core.Process through CoreV4.Process is safe (it checks Alpha+Beta exist).
func (c *Core) Process(ctx context.Context, req ProcessRequest) (ProcessResponse, error) {
	fmt.Println("[core] Process: start")

	// Defensive defaults (in case Build() was used instead of BuildWith()).
	if c.tracer == nil {
		fmt.Println("[core] tracer missing → using NoopTracer")
		c.tracer = NoopTracer{}
	}
	if c.metrics == nil {
		fmt.Println("[core] metrics missing → using NoopMetrics")
		c.metrics = NoopMetrics{}
	}

	// Required deps check (should already be enforced by facade).
	if c.alpha == nil || c.beta == nil {
		return ProcessResponse{}, fmt.Errorf("core wiring incomplete: alpha or beta is nil")
	}

	fmt.Println("[core] required deps present: Alpha, Beta")

	// ---- Tracing start
	ctx, end := c.tracer.StartSpan(ctx, "core.process")
	fmt.Println("[core] tracer.StartSpan called")

	// ---- Metrics
	c.metrics.Inc("core.process.calls")
	fmt.Println("[core] metrics.Inc(core.process.calls)")

	// ---- Alpha call
	fmt.Println("[core] calling Alpha.DoAlpha")
	a, err := c.alpha.DoAlpha(ctx, AlphaRequest{X: 1, Depth: 2})
	if err != nil {
		fmt.Println("[core] Alpha.DoAlpha returned error")
		end(err)
		return ProcessResponse{}, err
	}
	fmt.Printf("[core] Alpha.DoAlpha result: %+v\n", a)

	// ---- Beta call
	fmt.Println("[core] calling Beta.DoBeta")
	b, err := c.beta.DoBeta(ctx, BetaRequest{
		Input: fmt.Sprintf("%s:%d", req.OrderID, a.Value),
		Depth: 2,
	})
	if err != nil {
		fmt.Println("[core] Beta.DoBeta returned error")
		end(err)
		return ProcessResponse{}, err
	}
	fmt.Printf("[core] Beta.DoBeta result: %+v\n", b)

	// ---- Tracing end
	end(nil)
	fmt.Println("[core] tracer span ended successfully")

	resp := ProcessResponse{
		TimestampRFC3339: time.Now().Format(time.RFC3339),
		Env:              c.cfg.Env,
		Result:           b.Output,
	}

	fmt.Printf("[core] Process: completed → %+v\n", resp)
	return resp, nil
}
