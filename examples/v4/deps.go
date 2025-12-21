// odi/examples/v4/optional_deps.go
package v4

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Tracer is an optional dependency.
// If provided (via RegistryV2), Core will emit spans around work.
type Tracer interface {
	StartSpan(ctx context.Context, name string) (context.Context, func(err error))
}

// Metrics is an optional dependency.
// If provided (via RegistryV2), Core will emit counters.
type Metrics interface {
	Inc(name string)
}

// NoopTracer is used when no tracer is provided by registry.
// Keeps Core logic simple: trace calls always exist but do nothing.
type NoopTracer struct{}

func (NoopTracer) StartSpan(ctx context.Context, name string) (context.Context, func(err error)) {
	return ctx, func(err error) {}
}

// NoopMetrics is used when no metrics are provided.
type NoopMetrics struct{}

func (NoopMetrics) Inc(name string) {}

// -----------------------------------------------------------------------------
// Example implementations for the demo main + optional registry wiring.
// -----------------------------------------------------------------------------
//
// These are intentionally small and dependency-free, so the example stays easy
// to copy/paste into other repos.
//
// If you don't want these, delete them and provide real implementations via the
// RegistryV2 composition root (e.g. OpenTelemetry / Prometheus).

// PrintTracer is a very small tracer that logs span start/end to stdout.
// It is safe for concurrent use.
type PrintTracer struct {
	mu sync.Mutex
}

func NewPrintTracer() *PrintTracer { return &PrintTracer{} }

func (t *PrintTracer) StartSpan(ctx context.Context, name string) (context.Context, func(err error)) {
	t.mu.Lock()
	fmt.Println("[trace] start:", name)
	t.mu.Unlock()

	return ctx, func(err error) {
		t.mu.Lock()
		if err != nil {
			fmt.Println("[trace] end  :", name, "err=", err.Error())
		} else {
			fmt.Println("[trace] end  :", name, "ok")
		}
		t.mu.Unlock()
	}
}

// CounterMetrics is a tiny in-memory counter store.
// It is safe for concurrent use.
type CounterMetrics struct {
	mu   sync.Mutex
	vals map[string]int
}

func NewCounterMetrics() *CounterMetrics {
	return &CounterMetrics{vals: map[string]int{}}
}

func (m *CounterMetrics) Inc(name string) {
	m.mu.Lock()
	m.vals[name]++
	m.mu.Unlock()
}

// Snapshot returns a copy of the counters for printing/debugging.
func (m *CounterMetrics) Snapshot() map[string]int {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make(map[string]int, len(m.vals))
	for k, v := range m.vals {
		out[k] = v
	}
	return out
}

// FormatSnapshot is a helper used by examples to print counters consistently.
func FormatSnapshot(vals map[string]int) string {
	if len(vals) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(vals))
	for k, v := range vals {
		parts = append(parts, fmt.Sprintf("%s=%d", k, v))
	}
	// deterministic output for tests/examples
	sortStrings(parts)
	return "{" + strings.Join(parts, ", ") + "}"
}

func sortStrings(a []string) {
	// tiny insertion sort avoids importing "sort" for a single helper
	for i := 1; i < len(a); i++ {
		j := i
		for j > 0 && a[j-1] > a[j] {
			a[j-1], a[j] = a[j], a[j-1]
			j--
		}
	}
}
