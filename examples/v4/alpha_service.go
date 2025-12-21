package v4

import (
	"context"
	"fmt"

	"github.com/sghaida/odi/examples/v4/config"
)

//go:generate go run ../../cmd/di2 -spec specs/alpha.inject.json -out alpha_v4.gen.go
// Alpha depends on Beta (required) and participates in a cycle Alpha <-> Beta.
//
// The cycle is safe because business calls reduce Depth until reaching 0.
type Alpha struct {
	cfg  config.Config
	beta *Beta // required (cycle edge)
}

// NewAlpha is the constructor used by the generated facade (AlphaV4).
func NewAlpha(cfg config.Config) *Alpha { return &Alpha{cfg: cfg} }

// DoAlpha demonstrates:
// - Alpha needs Beta (cycle edge)
// - it calls Beta with Depth-1 to avoid infinite recursion
func (a *Alpha) DoAlpha(ctx context.Context, req AlphaRequest) (AlphaResponse, error) {
	if req.Depth <= 0 {
		return AlphaResponse{Value: req.X + 10}, nil
	}

	out, err := a.beta.DoBeta(ctx, BetaRequest{
		Input: fmt.Sprintf("alpha:%d", req.X),
		Depth: req.Depth - 1,
	})
	if err != nil {
		return AlphaResponse{}, err
	}

	return AlphaResponse{Value: req.X + len(out.Output)}, nil
}
