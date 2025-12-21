package di

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//
// -----------------------------------------------------------------------------
// NewMapRegistry / Provide
// -----------------------------------------------------------------------------

// TestNewMapRegistry_Empty verifies NewMapRegistry initializes a non-nil registry with an empty map.
func TestNewMapRegistry_Empty(t *testing.T) {
	t.Parallel()

	r := NewMapRegistry()
	require.NotNil(t, r)
	require.NotNil(t, r.items)
	assert.Len(t, r.items, 0)
}

// TestProvide_ChainsAndStores verifies Provide stores values and returns the same registry for chaining.
func TestProvide_ChainsAndStores(t *testing.T) {
	t.Parallel()

	r := NewMapRegistry()

	ret := r.Provide("a", 1).Provide("b", "x")
	require.Same(t, r, ret)

	gotA, okA := r.Get("a")
	require.True(t, okA)
	assert.Equal(t, 1, gotA)

	gotB, okB := r.Get("b")
	require.True(t, okB)
	assert.Equal(t, "x", gotB)
}

//
// -----------------------------------------------------------------------------
// Get
// -----------------------------------------------------------------------------

// TestGet_Missing verifies Get returns (nil,false) for missing keys.
func TestGet_Missing(t *testing.T) {
	t.Parallel()

	r := NewMapRegistry()
	got, ok := r.Get("missing")
	assert.False(t, ok)
	assert.Nil(t, got)
}

// TestGet_Present verifies Get returns the stored value for existing keys.
func TestGet_Present(t *testing.T) {
	t.Parallel()

	r := NewMapRegistry().Provide("k", 123)
	got, ok := r.Get("k")
	require.True(t, ok)
	assert.Equal(t, 123, got)
}

//
// -----------------------------------------------------------------------------
// Resolve
// -----------------------------------------------------------------------------

// TestResolve_Present verifies Resolve returns the stored value and ok=true.
func TestResolve_Present(t *testing.T) {
	t.Parallel()

	r := NewMapRegistry().Provide("k", "v")

	val, ok, err := r.Resolve(struct{}{}, "k")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "v", val)
}

// TestResolve_Missing verifies Resolve returns (nil,false,nil) for missing keys.
func TestResolve_Missing(t *testing.T) {
	t.Parallel()

	r := NewMapRegistry()

	val, ok, err := r.Resolve(nil, "missing")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Nil(t, val)
}

// TestResolve_IgnoresCfg verifies cfg is ignored (value returned is the same regardless of cfg).
func TestResolve_IgnoresCfg(t *testing.T) {
	t.Parallel()

	r := NewMapRegistry().Provide("k", 42)

	val1, ok1, err1 := r.Resolve(nil, "k")
	val2, ok2, err2 := r.Resolve(map[string]any{"x": "y"}, "k")

	require.NoError(t, err1)
	require.NoError(t, err2)
	require.True(t, ok1)
	require.True(t, ok2)
	assert.Equal(t, 42, val1)
	assert.Equal(t, val1, val2)
}

// TestResolve_RecoversFromPanic verifies Resolve converts internal panics into errors.
// We trigger a panic via a nil receiver, which panics when accessing r.items in Resolve.
func TestResolve_RecoversFromPanic(t *testing.T) {
	t.Parallel()

	var r *MapRegistry // nil receiver

	val, ok, err := r.Resolve(nil, "k")

	require.Error(t, err)
	assert.False(t, ok)
	assert.Nil(t, val)

	assert.True(t, errors.Is(err, ErrRegistryPanic), "expected ErrRegistryPanic wrapping, got: %v", err)
	assert.Contains(t, err.Error(), "registry: panic during Resolve")
}

//
// -----------------------------------------------------------------------------
// MustGet
// -----------------------------------------------------------------------------

// TestMustGet_Present verifies MustGet returns the stored value.
func TestMustGet_Present(t *testing.T) {
	t.Parallel()

	r := NewMapRegistry().Provide("k", "v")
	assert.Equal(t, "v", r.MustGet("k"))
}

// TestMustGet_Missing verifies MustGet panics with a helpful message when key is missing.
func TestMustGet_Missing(t *testing.T) {
	t.Parallel()

	r := NewMapRegistry()

	require.PanicsWithError(t, `di: registry missing key "missing"`, func() {
		_ = r.MustGet("missing")
	})
}
