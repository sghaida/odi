package di_test

import (
	"errors"
	"testing"

	di2 "github.com/sghaida/odi/examples/di"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Init / Value
func TestInitAndValue(t *testing.T) {
	t.Parallel()

	svc := di2.Init(func() *di2.UserService { return &di2.UserService{} })

	require.NotNil(t, svc)
	require.NotNil(t, svc.Value())
	require.NotNil(t, svc.Deps)
	assert.Empty(t, svc.Deps)
}

// DependencyKey helper
func TestKey(t *testing.T) {
	t.Parallel()

	assert.Equal(t, di2.DependencyKey("db"), di2.Key("db"))
}

// With / WithAll
func TestWith_NilInjector_NoOp(t *testing.T) {
	t.Parallel()

	svc := di2.Init(func() *di2.UserService { return &di2.UserService{} })
	before := svc.Value()

	got, err := svc.With(nil)
	require.NoError(t, err)
	assert.Same(t, svc, got)
	assert.Same(t, before, got.Value())
}

func TestWithAll_AppliesInOrderAndStopsOnError(t *testing.T) {
	t.Parallel()

	dbKey := di2.Key("db")
	logKey := di2.Key("logger")

	db := di2.Init(func() *di2.DB { return &di2.DB{DSN: "postgres://"} })
	logger := di2.Init(func() *di2.Logger { return &di2.Logger{Level: "info"} })

	user := di2.Init(func() *di2.UserService { return &di2.UserService{} })

	injDB := di2.Injecting(dbKey, db, func(u *di2.UserService, d *di2.DB) { u.DB = d })
	injLogger := di2.Injecting(logKey, logger, func(u *di2.UserService, l *di2.Logger) { u.Logger = l })

	_, err := user.WithAll(injDB, injDB, injLogger)
	require.Error(t, err)

	var dup di2.DuplicateKeyError
	require.True(t, errors.As(err, &dup))
	assert.Equal(t, dbKey, dup.Key)

	// DB applied once
	require.NotNil(t, user.Value().DB)
	// Logger not applied due to early stop
	assert.Nil(t, user.Value().Logger)

	_, ok := user.Deps[dbKey]
	assert.True(t, ok)
	_, ok = user.Deps[logKey]
	assert.False(t, ok)
}

// Injecting – error cases
func TestInjecting_Errors(t *testing.T) {
	t.Parallel()

	key := di2.Key("db")

	validDep := di2.Init(func() *di2.DB { return &di2.DB{} })
	validBind := func(u *di2.UserService, d *di2.DB) { u.DB = d }

	cases := []struct {
		name      string
		targetSvc *di2.Service[di2.UserService]
		depSvc    *di2.Service[di2.DB]
		bind      func(*di2.UserService, *di2.DB)

		wantIs  error
		wantAs  any
		wantKey di2.DependencyKey
	}{
		{
			name:      "nil target service",
			targetSvc: nil,
			depSvc:    validDep,
			bind:      validBind,
			wantIs:    di2.ErrNilTarget,
		},
		{
			name:      "nil target value",
			targetSvc: &di2.Service[di2.UserService]{Val: nil, Deps: map[di2.DependencyKey]any{}},
			depSvc:    validDep,
			bind:      validBind,
			wantIs:    di2.ErrNilTarget,
		},
		{
			name:      "nil dependency service",
			targetSvc: di2.Init(func() *di2.UserService { return &di2.UserService{} }),
			depSvc:    nil,
			bind:      validBind,
			wantAs:    (*di2.NilDependencyServiceError)(nil),
			wantKey:   key,
		},
		{
			name:      "nil dependency value",
			targetSvc: di2.Init(func() *di2.UserService { return &di2.UserService{} }),
			depSvc:    &di2.Service[di2.DB]{Val: nil, Deps: map[di2.DependencyKey]any{}},
			bind:      validBind,
			wantAs:    (*di2.NilDependencyServiceError)(nil),
			wantKey:   key,
		},
		{
			name:      "nil bind function",
			targetSvc: di2.Init(func() *di2.UserService { return &di2.UserService{} }),
			depSvc:    validDep,
			bind:      nil,
			wantAs:    (*di2.NilBindError)(nil),
			wantKey:   key,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			inj := di2.Injecting(key, tc.depSvc, tc.bind)
			err := inj(tc.targetSvc)
			require.Error(t, err)

			if tc.wantIs != nil {
				require.True(t, errors.Is(err, tc.wantIs))
				return
			}

			switch tc.wantAs.(type) {
			case *di2.NilDependencyServiceError:
				var got di2.NilDependencyServiceError
				require.True(t, errors.As(err, &got))
				assert.Equal(t, tc.wantKey, got.Key)

			case *di2.NilBindError:
				var got di2.NilBindError
				require.True(t, errors.As(err, &got))
				assert.Equal(t, tc.wantKey, got.Key)

			default:
				t.Fatalf("misconfigured test case")
			}
		})
	}
}

// Injecting – success, Deps map creation branch, and duplicate detection
func TestInjecting_SuccessAndDepsMapCreationAndDuplicate(t *testing.T) {
	t.Parallel()

	dbKey := di2.Key("db")

	db := di2.Init(func() *di2.DB { return &di2.DB{DSN: "mysql://"} })

	// cover the branch: if s.Deps == nil { s.Deps = make(...) }
	targetNilDeps := &di2.Service[di2.UserService]{Val: &di2.UserService{}, Deps: nil}
	inj := di2.Injecting(dbKey, db, func(u *di2.UserService, d *di2.DB) { u.DB = d })

	require.NoError(t, inj(targetNilDeps))
	require.NotNil(t, targetNilDeps.Deps)
	assert.True(t, targetNilDeps.Has(dbKey))
	require.NotNil(t, targetNilDeps.Val.DB)
	assert.Equal(t, "mysql://", targetNilDeps.Val.DB.DSN)

	// Now cover duplicate detection via the normal With path
	user := di2.Init(func() *di2.UserService { return &di2.UserService{} })
	_, err := user.With(inj)
	require.NoError(t, err)

	raw, ok := user.GetAny(dbKey)
	require.True(t, ok)
	got, ok := raw.(*di2.DB)
	require.True(t, ok)
	assert.Same(t, db.Value(), got)

	_, err = user.With(inj)
	require.Error(t, err)

	var dup di2.DuplicateKeyError
	require.True(t, errors.As(err, &dup))
	assert.Equal(t, dbKey, dup.Key)
}

// Accessors – Has/GetAny/GetAs/TryGetAs/MustGetAs, plus nil/guard branches
func TestAccessors_GetAsTryGetAsMustGetAs(t *testing.T) {
	t.Parallel()

	dbKey := di2.Key("db")
	basketKey := di2.Key("basket")

	db := di2.Init(func() *di2.DB { return &di2.DB{DSN: "sqlite"} })
	basket := di2.Init(func() *di2.BasketService { return &di2.BasketService{} })
	user := di2.Init(func() *di2.UserService { return &di2.UserService{} })

	_, err := user.WithAll(
		di2.Injecting(dbKey, db, func(u *di2.UserService, d *di2.DB) { u.DB = d }),
		di2.Injecting(basketKey, basket, func(u *di2.UserService, b *di2.BasketService) { u.Basket = b }),
	)
	require.NoError(t, err)

	// GetAs success
	gotDB, ok := di2.GetAs[di2.UserService, di2.DB](user, dbKey)
	require.True(t, ok)
	assert.Same(t, db.Value(), gotDB)

	// MustGetAs success (covers `return d`)
	gotMust := di2.MustGetAs[di2.UserService, di2.DB](user, dbKey)
	require.NotNil(t, gotMust)
	assert.Same(t, db.Value(), gotMust)

	// TryGetAs missing
	_, err = di2.TryGetAs[di2.UserService, di2.DB](user, di2.Key("missing"))
	require.Error(t, err)

	// MustGetAs panic on wrong key/type
	assert.Panics(t, func() {
		_ = di2.MustGetAs[di2.UserService, di2.DB](user, basketKey)
	})
}

func TestAccessors_GetAsAndHas_Guards(t *testing.T) {
	t.Parallel()

	dbKey := di2.Key("db")

	type guardCase struct {
		name string
		svc  *di2.Service[di2.UserService]
	}

	cases := []guardCase{
		{name: "nil service", svc: nil},
		{name: "nil deps", svc: &di2.Service[di2.UserService]{Val: &di2.UserService{}, Deps: nil}},
		{name: "missing key", svc: &di2.Service[di2.UserService]{Val: &di2.UserService{}, Deps: map[di2.DependencyKey]any{}}},
		{name: "raw nil value", svc: &di2.Service[di2.UserService]{Val: &di2.UserService{}, Deps: map[di2.DependencyKey]any{dbKey: nil}}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// covers GetAs guards:
			// - s==nil or s.Deps==nil
			// - !ok || raw==nil
			got, ok := di2.GetAs[di2.UserService, di2.DB](tc.svc, dbKey)
			assert.Nil(t, got)
			assert.False(t, ok)

			// covers Has guards (nil service / nil deps return false)
			if tc.svc == nil || tc.svc.Deps == nil {
				var has bool
				if tc.svc == nil {
					has = (*di2.Service[di2.UserService])(nil).Has(dbKey)
				} else {
					has = tc.svc.Has(dbKey)
				}
				assert.False(t, has)
			}

			// covers GetAny guards (nil service / nil deps return nil,false)
			if tc.svc == nil || tc.svc.Deps == nil {
				var v any
				var ok2 bool
				if tc.svc == nil {
					v, ok2 = (*di2.Service[di2.UserService])(nil).GetAny(dbKey)
				} else {
					v, ok2 = tc.svc.GetAny(dbKey)
				}
				assert.Nil(t, v)
				assert.False(t, ok2)
			}
		})
	}
}

func TestAccessors_TryGetAs_Table(t *testing.T) {
	t.Parallel()

	dbKey := di2.Key("db")
	loggerKey := di2.Key("logger")

	// success setup: inject DB so TryGetAs hits `return d, nil`
	db := di2.Init(func() *di2.DB { return &di2.DB{DSN: "postgres://prod"} })
	user := di2.Init(func() *di2.UserService { return &di2.UserService{} })
	_, err := user.With(di2.Injecting(dbKey, db, func(u *di2.UserService, d *di2.DB) { u.DB = d }))
	require.NoError(t, err)

	cases := []struct {
		name      string
		svc       *di2.Service[di2.UserService]
		key       di2.DependencyKey
		wantErrAs any
		wantType  string
		wantOK    bool
	}{
		{
			name:      "nil service -> missing",
			svc:       nil,
			key:       dbKey,
			wantErrAs: di2.MissingDependencyError{},
		},
		{
			name:      "nil deps -> missing",
			svc:       &di2.Service[di2.UserService]{Val: &di2.UserService{}, Deps: nil},
			key:       dbKey,
			wantErrAs: di2.MissingDependencyError{},
		},
		{
			name: "wrong type -> wrong type error",
			svc: &di2.Service[di2.UserService]{Val: &di2.UserService{}, Deps: map[di2.DependencyKey]any{
				loggerKey: &di2.Logger{Level: "info"},
			}},
			key:       loggerKey,
			wantErrAs: di2.WrongTypeDependencyError{},
			wantType:  "*di.Logger",
		},
		{
			name:   "success -> returns value and nil error",
			svc:    user,
			key:    dbKey,
			wantOK: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := di2.TryGetAs[di2.UserService, di2.DB](tc.svc, tc.key)

			if tc.wantOK {
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, "postgres://prod", got.DSN)
				return
			}

			require.Error(t, err)

			switch tc.wantErrAs.(type) {
			case di2.MissingDependencyError:
				var me di2.MissingDependencyError
				require.True(t, errors.As(err, &me))
				assert.Equal(t, tc.key, me.Key)
				assert.Equal(t, `di: dependency "`+string(tc.key)+`" missing`, me.Error())

			case di2.WrongTypeDependencyError:
				var we di2.WrongTypeDependencyError
				require.True(t, errors.As(err, &we))
				assert.Equal(t, tc.key, we.Key)
				assert.Equal(t, tc.wantType, we.GotType)
				assert.Contains(t, we.Error(), `dependency "`+string(tc.key)+`" has wrong type`)
				assert.Contains(t, we.Error(), tc.wantType)

			default:
				t.Fatalf("misconfigured test case")
			}
		})
	}
}

// Clone – branches and copy behavior
func TestClone_BranchesAndCopyBehavior(t *testing.T) {
	t.Parallel()

	// covers: if s == nil { return nil }
	var nilSvc *di2.Service[di2.UserService]
	assert.Nil(t, nilSvc.Clone())

	// covers: else branch where len(s.Deps)==0 -> make(map...)
	empty := &di2.Service[di2.UserService]{Val: &di2.UserService{}, Deps: map[di2.DependencyKey]any{}}
	cpEmpty := empty.Clone()
	require.NotNil(t, cpEmpty)
	require.NotNil(t, cpEmpty.Deps)
	assert.Empty(t, cpEmpty.Deps)
	cpEmpty.Deps[di2.Key("x")] = "y"
	_, ok := empty.Deps[di2.Key("x")]
	assert.False(t, ok)

	// covers: copy deps map but share Val
	key := di2.Key("db")
	db := di2.Init(func() *di2.DB { return &di2.DB{DSN: "clone"} })
	user := di2.Init(func() *di2.UserService { return &di2.UserService{} })
	_, err := user.With(di2.Injecting(key, db, func(u *di2.UserService, d *di2.DB) { u.DB = d }))
	require.NoError(t, err)

	cp := user.Clone()
	require.NotNil(t, cp)
	assert.Same(t, user.Val, cp.Val)
	cp.Deps[di2.Key("extra")] = "x"
	_, ok = user.Deps[di2.Key("extra")]
	assert.False(t, ok)
}

// Errors – ensure Error() strings are covered in one place
func TestErrors_StringAndTyping(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "DuplicateKeyError",
			err:  di2.DuplicateKeyError{Key: di2.Key("db")},
			want: `di: duplicate dependency key "db"`,
		},
		{
			name: "MissingDependencyError",
			err:  di2.MissingDependencyError{Key: di2.Key("db")},
			want: `di: dependency "db" missing`,
		},
		{
			name: "WrongTypeDependencyError",
			err:  di2.WrongTypeDependencyError{Key: di2.Key("logger"), GotType: "*di.Logger"},
			want: `di: dependency "logger" has wrong type (*di.Logger)`,
		},
		{
			name: "NilDependencyServiceError",
			err:  di2.NilDependencyServiceError{Key: di2.Key("db")},
			want: `di: nil dependency service for key "db"`,
		},
		{
			name: "NilBindError",
			err:  di2.NilBindError{Key: di2.Key("db")},
			want: `di: nil bind function for key "db"`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, tc.err.Error())
		})
	}
}
