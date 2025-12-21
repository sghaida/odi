package v4

import (
	"context"
	"fmt"

	"github.com/sghaida/odi/examples/v4/config"
)

//go:generate go run ../../cmd/di2 -spec specs/beta.inject.json -out beta_v4.gen.go

// Beta depends on Alpha (required) and participates in a cycle Alpha <-> Beta.
type Beta struct {
	cfg   config.Config
	alpha *Alpha // required (cycle edge)
}

// NewBeta is the constructor used by the generated facade (BetaV4).
func NewBeta(cfg config.Config) *Beta { return &Beta{cfg: cfg} }

// DoBeta demonstrates:
// - Beta needs Alpha (cycle edge)
// - it calls Alpha with Depth-1 to avoid infinite recursion
func (b *Beta) DoBeta(ctx context.Context, req BetaRequest) (BetaResponse, error) {
	if req.Depth <= 0 {
		return BetaResponse{Output: "beta:base:" + req.Input}, nil
	}

	a, err := b.alpha.DoAlpha(ctx, AlphaRequest{
		X:     len(req.Input),
		Depth: req.Depth - 1,
	})
	if err != nil {
		return BetaResponse{}, err
	}

	return BetaResponse{Output: fmt.Sprintf("beta:%s:n=%d", req.Input, a.Value)}, nil
}
