package di_test

import (
	"testing"

	di2 "github.com/sghaida/odi/examples/di"
	"github.com/stretchr/testify/require"
)

func TestNew_ServiceV2_Table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "New(DB) constructs value",
			run: func(t *testing.T) {
				t.Parallel()

				s := di2.New(func() *di2.DB { return &di2.DB{DSN: "postgres://prod"} })
				require.NotNil(t, s.Val)
				require.Equal(t, "postgres://prod", s.Val.DSN)
			},
		},
		{
			name: "New(Logger) constructs value",
			run: func(t *testing.T) {
				t.Parallel()

				s := di2.New(func() *di2.Logger { return &di2.Logger{Level: "info"} })
				require.NotNil(t, s.Val)
				require.Equal(t, "info", s.Val.Level)
			},
		},
		{
			name: "New(BasketService) constructs value",
			run: func(t *testing.T) {
				t.Parallel()

				s := di2.New(func() *di2.BasketService { return &di2.BasketService{} })
				require.NotNil(t, s.Val)
				require.Nil(t, s.Val.DB)
				require.Nil(t, s.Val.Logger)
			},
		},
		{
			name: "Manual wiring: BasketService depends on DB + Logger",
			run: func(t *testing.T) {
				t.Parallel()

				// Construct deps
				db := di2.New(func() *di2.DB { return &di2.DB{DSN: "postgres://prod"} })
				logger := di2.New(func() *di2.Logger { return &di2.Logger{Level: "debug"} })

				// Construct service
				basket := di2.New(func() *di2.BasketService { return &di2.BasketService{} })

				// V2 "DI": manual pointer wiring
				basket.Val.DB = db.Val
				basket.Val.Logger = logger.Val

				// Validate wiring uses the same instances
				require.Same(t, db.Val, basket.Val.DB)
				require.Same(t, logger.Val, basket.Val.Logger)

				// Mutations flow through shared pointers (proves it's the same instance)
				db.Val.DSN = "postgres://changed"
				require.Equal(t, "postgres://changed", basket.Val.DB.DSN)

				logger.Val.Level = "warn"
				require.Equal(t, "warn", basket.Val.Logger.Level)
			},
		},
		{
			name: "Copying ServiceV2 keeps pointer identity",
			run: func(t *testing.T) {
				t.Parallel()

				db := di2.New(func() *di2.DB { return &di2.DB{DSN: "sqlite://"} })
				db2 := db // copy the container

				require.Same(t, db.Val, db2.Val)

				db.Val.DSN = "sqlite://changed"
				require.Equal(t, "sqlite://changed", db2.Val.DSN)
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tc.run(t)
		})
	}
}
