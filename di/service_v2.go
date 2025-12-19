package di


// ServiceV2 Generic service instance container (no dep-state here anymore; state is in wrappers)
type ServiceV2[T any] struct {
	Val *T
}

func New[T any](ctor func() *T) ServiceV2[T] {
	return ServiceV2[T]{ Val: ctor()}
}
