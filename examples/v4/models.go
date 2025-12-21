package v4

// ProcessRequest is the input DTO for Core.Process.
type ProcessRequest struct {
	OrderID string
}

// ProcessResponse is the output DTO for Core.Process.
type ProcessResponse struct {
	TimestampRFC3339 string
	Env              string
	Result           string
}

// AlphaRequest is the input DTO for Alpha.DoAlpha.
// Depth is used to prevent infinite recursion in the Alpha<->Beta cycle.
type AlphaRequest struct {
	X     int
	Depth int
}

// AlphaResponse is the output DTO for Alpha.DoAlpha.
type AlphaResponse struct {
	Value int
}

// BetaRequest is the input DTO for Beta.DoBeta.
// Depth is used to prevent infinite recursion in the Alpha<->Beta cycle.
type BetaRequest struct {
	Input string
	Depth int
}

// BetaResponse is the output DTO for Beta.DoBeta.
type BetaResponse struct {
	Output string
}
