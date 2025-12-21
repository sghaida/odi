// odi/examples/v4/main/main.go
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/sghaida/odi/di"
	v4 "github.com/sghaida/odi/examples/v4"
	"github.com/sghaida/odi/examples/v4/config"
)

// This example shows two ways to wire the same app:
//
// Graph wiring (recommended for production):
//   - one function builds the whole object graph consistently
//   - wiring is centralized and easy to review
//
// Manual wiring (useful for tests or small experiments):
//   - you build each service builder yourself and inject deps explicitly
//
// Prerequisites / generation:
//   - Ensure you generated the service facades + graph:
//     go generate ./...
//
// Running:
//   - go run ./odi/examples/v4/main
func main() {
	// -------------------------------------------------------------------------
	// Step 1: Load config (project-specific)
	// -------------------------------------------------------------------------
	//
	// Why:
	// - In these examples, services are constructed with config.Config.
	// - Generated builders and graph constructors expect config when config is enabled.
	cfg, err := config.LoadFromEnv()
	if err != nil {
		_, err := fmt.Fprintln(os.Stderr, "config.LoadFromEnv failed:", err)
		if err != nil {
			return
		}
		os.Exit(1)
	}

	// -------------------------------------------------------------------------
	// Step 2: Create a context for the demo
	// -------------------------------------------------------------------------
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// -------------------------------------------------------------------------
	// Step 3: Optional deps via Registry (Tracer + Metrics)
	// -------------------------------------------------------------------------
	//
	// Your generated builders call:
	//   reg.Resolve(cfg, "v4.tracer")
	//   reg.Resolve(cfg, "v4.metrics")
	//
	// The registry is optional; if you pass nil, optional deps will be missing and:
	// - if DefaultExpr is configured in spec, it will be used
	// - otherwise the optional will remain unset
	reg := di.NewMapRegistry().
		Provide("v4.tracer", v4.NewPrintTracer()).
		Provide("v4.metrics", v4.NewCounterMetrics())

	// -------------------------------------------------------------------------
	// Step 4: Graph wiring (recommended)
	// -------------------------------------------------------------------------
	//
	// BuildAppV4 is generated from your graph.json and does the following:
	// - creates Alpha/Beta/Core builders
	// - wires Alpha <-> Beta cycle using UnsafeImpl() only for wiring
	// - injects Alpha & Beta into Core
	// - calls BuildWith(reg) (because buildWithRegistry=true)
	//
	// This is the "composition root" pattern.
	app, err := v4.BuildAppV4(cfg, reg)
	if err != nil {
		_, err := fmt.Fprintln(os.Stderr, "v4.BuildAppV4 failed:", err)
		if err != nil {
			return
		}
		os.Exit(1)
	}

	// -------------------------------------------------------------------------
	// Step 5: Use the built services
	// -------------------------------------------------------------------------
	//
	// IMPORTANT:
	// - These calls must match your *actual* service method signatures.
	// - If you changed your specs to use (ctx, req) signatures, call with req.
	alphaOut, err := app.Alpha.DoAlpha(ctx, v4.AlphaRequest{X: 7, Depth: 3})
	if err != nil {
		_, err := fmt.Fprintln(os.Stderr, "Alpha.DoAlpha failed:", err)
		if err != nil {
			return
		}
		os.Exit(1)
	}
	fmt.Println("alpha:", alphaOut)

	coreOut, err := app.Core.Process(ctx, v4.ProcessRequest{OrderID: "order-id-test"})
	if err != nil {
		_, err := fmt.Fprintln(os.Stderr, "Core.DoCore failed:", err)
		if err != nil {
			return
		}
		os.Exit(1)
	}
	fmt.Println("core :", coreOut)

	// Show optional dep metrics snapshot (only works if your Core increments metrics).
	if m, ok := reg.MustGet("v4.metrics").(*v4.CounterMetrics); ok {
		fmt.Println("metrics:", v4.FormatSnapshot(m.Snapshot()))
	}

	// -------------------------------------------------------------------------
	// Step 6: Manual wiring (individual injections usage)
	// -------------------------------------------------------------------------
	//
	// When to use:
	// - unit tests (inject fakes)
	// - very small graphs
	// - experimenting with different wiring policies
	//
	// How it works:
	// - build builders
	// - inject required deps (including cycles via UnsafeImpl())
	// - BuildWith(reg) to apply optional deps
	alphaService := v4.NewAlphaV4(cfg)
	betaService := v4.NewBetaV4(cfg)

	// Cycle wiring: use UnsafeImpl() ONLY for wiring (not for calling methods).
	alphaService.InjectBeta(betaService.UnsafeImpl())
	betaService.InjectAlpha(alphaService.UnsafeImpl())

	coreService := v4.NewCoreV4(cfg).
		InjectAlpha(alphaService.UnsafeImpl()).
		InjectBeta(betaService.UnsafeImpl())

	alphaSvc, err := alphaService.BuildWith(reg)
	if err != nil {
		_, err := fmt.Fprintln(os.Stderr, "manual alpha build failed:", err)
		if err != nil {
			return
		}
		os.Exit(1)
	}
	_, err = betaService.BuildWith(reg)
	if err != nil {
		_, err := fmt.Fprintln(os.Stderr, "manual beta build failed:", err)
		if err != nil {
			return
		}
		os.Exit(1)
	}
	coreSvc, err := coreService.BuildWith(reg)
	if err != nil {
		_, err := fmt.Fprintln(os.Stderr, "manual core build failed:", err)
		if err != nil {
			return
		}
		os.Exit(1)
	}

	// Sanity calls on manually built services.
	_ = alphaSvc
	_ = coreSvc
	fmt.Println("manual wiring: OK")
}
