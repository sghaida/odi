package di_test

import (
	"errors"
	"testing"

	"github.com/sghaida/odi/di"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Init / Value
func TestInitAndValue(t *testing.T) {
	t.Parallel()

	svc := di.Init(func() *di.UserService { return &di.UserService{} })

	require.NotNil(t, svc)
	require.NotNil(t, svc.Value())
	require.NotNil(t, svc.Deps)
	assert.Empty(t, svc.Deps)
}

// DependencyKey helper
func TestKey(t *testing.T) {
	t.Parallel()

	assert.Equal(t, di.DependencyKey("db"), di.Key("db"))
}

// With / WithAll
func TestWith_NilInjector_NoOp(t *testing.T) {
	t.Parallel()

	svc := di.Init(func() *di.UserService { return &di.UserService{} })
	before := svc.Value()

	got, err := svc.With(nil)
	require.NoError(t, err)
	assert.Same(t, svc, got)
	assert.Same(t, before, got.Value())
}

func TestWithAll_AppliesInOrderAndStopsOnError(t *testing.T) {
	t.Parallel()

	dbKey := di.Key("db")
	logKey := di.Key("logger")

	db := di.Init(func() *di.DB { return &di.DB{DSN: "postgres://"} })
	logger := di.Init(func() *di.Logger { return &di.Logger{Level: "info"} })

	user := di.Init(func() *di.UserService { return &di.UserService{} })

	injDB := di.Injecting(dbKey, db, func(u *di.UserService, d *di.DB) { u.DB = d })
	injLogger := di.Injecting(logKey, logger, func(u *di.UserService, l *di.Logger) { u.Logger = l })

	_, err := user.WithAll(injDB, injDB, injLogger)
	require.Error(t, err)

	var dup di.DuplicateKeyError
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

	key := di.Key("db")

	validDep := di.Init(func() *di.DB { return &di.DB{} })
	validBind := func(u *di.UserService, d *di.DB) { u.DB = d }

	cases := []struct {
		name      string
		targetSvc *di.Service[di.UserService]
		depSvc    *di.Service[di.DB]
		bind      func(*di.UserService, *di.DB)

		wantIs  error
		wantAs  any
		wantKey di.DependencyKey
	}{
		{
			name:      "nil target service",
			targetSvc: nil,
			depSvc:    validDep,
			bind:      validBind,
			wantIs:    di.ErrNilTarget,
		},
		{
			name:      "nil target value",
			targetSvc: &di.Service[di.UserService]{Val: nil, Deps: map[di.DependencyKey]any{}},
			depSvc:    validDep,
			bind:      validBind,
			wantIs:    di.ErrNilTarget,
		},
		{
			name:      "nil dependency service",
			targetSvc: di.Init(func() *di.UserService { return &di.UserService{} }),
			depSvc:    nil,
			bind:      validBind,
			wantAs:    (*di.NilDependencyServiceError)(nil),
			wantKey:   key,
		},
		{
			name:      "nil dependency value",
			targetSvc: di.Init(func() *di.UserService { return &di.UserService{} }),
			depSvc:    &di.Service[di.DB]{Val: nil, Deps: map[di.DependencyKey]any{}},
			bind:      validBind,
			wantAs:    (*di.NilDependencyServiceError)(nil),
			wantKey:   key,
		},
		{
			name:      "nil bind function",
			targetSvc: di.Init(func() *di.UserService { return &di.UserService{} }),
			depSvc:    validDep,
			bind:      nil,
			wantAs:    (*di.NilBindError)(nil),
			wantKey:   key,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			inj := di.Injecting(key, tc.depSvc, tc.bind)
			err := inj(tc.targetSvc)
			require.Error(t, err)

			if tc.wantIs != nil {
				require.True(t, errors.Is(err, tc.wantIs))
				return
			}

			switch tc.wantAs.(type) {
			case *di.NilDependencyServiceError:
				var got di.NilDependencyServiceError
				require.True(t, errors.As(err, &got))
				assert.Equal(t, tc.wantKey, got.Key)

			case *di.NilBindError:
				var got di.NilBindError
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

	dbKey := di.Key("db")

	db := di.Init(func() *di.DB { return &di.DB{DSN: "mysql://"} })

	// cover the branch: if s.Deps == nil { s.Deps = make(...) }
	targetNilDeps := &di.Service[di.UserService]{Val: &di.UserService{}, Deps: nil}
	inj := di.Injecting(dbKey, db, func(u *di.UserService, d *di.DB) { u.DB = d })

	require.NoError(t, inj(targetNilDeps))
	require.NotNil(t, targetNilDeps.Deps)
	assert.True(t, targetNilDeps.Has(dbKey))
	require.NotNil(t, targetNilDeps.Val.DB)
	assert.Equal(t, "mysql://", targetNilDeps.Val.DB.DSN)

	// Now cover duplicate detection via the normal With path
	user := di.Init(func() *di.UserService { return &di.UserService{} })
	_, err := user.With(inj)
	require.NoError(t, err)

	raw, ok := user.GetAny(dbKey)
	require.True(t, ok)
	got, ok := raw.(*di.DB)
	require.True(t, ok)
	assert.Same(t, db.Value(), got)

	_, err = user.With(inj)
	require.Error(t, err)

	var dup di.DuplicateKeyError
	require.True(t, errors.As(err, &dup))
	assert.Equal(t, dbKey, dup.Key)
}

// Accessors – Has/GetAny/GetAs/TryGetAs/MustGetAs, plus nil/guard branches
func TestAccessors_GetAsTryGetAsMustGetAs(t *testing.T) {
	t.Parallel()

	dbKey := di.Key("db")
	basketKey := di.Key("basket")

	db := di.Init(func() *di.DB { return &di.DB{DSN: "sqlite"} })
	basket := di.Init(func() *di.BasketService { return &di.BasketService{} })
	user := di.Init(func() *di.UserService { return &di.UserService{} })

	_, err := user.WithAll(
		di.Injecting(dbKey, db, func(u *di.UserService, d *di.DB) { u.DB = d }),
		di.Injecting(basketKey, basket, func(u *di.UserService, b *di.BasketService) { u.Basket = b }),
	)
	require.NoError(t, err)

	// GetAs success
	gotDB, ok := di.GetAs[di.UserService, di.DB](user, dbKey)
	require.True(t, ok)
	assert.Same(t, db.Value(), gotDB)

	// MustGetAs success (covers `return d`)
	gotMust := di.MustGetAs[di.UserService, di.DB](user, dbKey)
	require.NotNil(t, gotMust)
	assert.Same(t, db.Value(), gotMust)

	// TryGetAs missing
	_, err = di.TryGetAs[di.UserService, di.DB](user, di.Key("missing"))
	require.Error(t, err)

	// MustGetAs panic on wrong key/type
	assert.Panics(t, func() {
		_ = di.MustGetAs[di.UserService, di.DB](user, basketKey)
	})
}

func TestAccessors_GetAsAndHas_Guards(t *testing.T) {
	t.Parallel()

	dbKey := di.Key("db")

	type guardCase struct {
		name string
		svc  *di.Service[di.UserService]
	}

	cases := []guardCase{
		{name: "nil service", svc: nil},
		{name: "nil deps", svc: &di.Service[di.UserService]{Val: &di.UserService{}, Deps: nil}},
		{name: "missing key", svc: &di.Service[di.UserService]{Val: &di.UserService{}, Deps: map[di.DependencyKey]any{}}},
		{name: "raw nil value", svc: &di.Service[di.UserService]{Val: &di.UserService{}, Deps: map[di.DependencyKey]any{dbKey: nil}}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// covers GetAs guards:
			// - s==nil or s.Deps==nil
			// - !ok || raw==nil
			got, ok := di.GetAs[di.UserService, di.DB](tc.svc, dbKey)
			assert.Nil(t, got)
			assert.False(t, ok)

			// covers Has guards (nil service / nil deps return false)
			if tc.svc == nil || tc.svc.Deps == nil {
				var has bool
				if tc.svc == nil {
					has = (*di.Service[di.UserService])(nil).Has(dbKey)
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
					v, ok2 = (*di.Service[di.UserService])(nil).GetAny(dbKey)
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

	dbKey := di.Key("db")
	loggerKey := di.Key("logger")

	// success setup: inject DB so TryGetAs hits `return d, nil`
	db := di.Init(func() *di.DB { return &di.DB{DSN: "postgres://prod"} })
	user := di.Init(func() *di.UserService { return &di.UserService{} })
	_, err := user.With(di.Injecting(dbKey, db, func(u *di.UserService, d *di.DB) { u.DB = d }))
	require.NoError(t, err)

	cases := []struct {
		name      string
		svc       *di.Service[di.UserService]
		key       di.DependencyKey
		wantErrAs any
		wantType  string
		wantOK    bool
	}{
		{
			name:      "nil service -> missing",
			svc:       nil,
			key:       dbKey,
			wantErrAs: di.MissingDependencyError{},
		},
		{
			name:      "nil deps -> missing",
			svc:       &di.Service[di.UserService]{Val: &di.UserService{}, Deps: nil},
			key:       dbKey,
			wantErrAs: di.MissingDependencyError{},
		},
		{
			name: "wrong type -> wrong type error",
			svc: &di.Service[di.UserService]{Val: &di.UserService{}, Deps: map[di.DependencyKey]any{
				loggerKey: &di.Logger{Level: "info"},
			}},
			key:       loggerKey,
			wantErrAs: di.WrongTypeDependencyError{},
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

			got, err := di.TryGetAs[di.UserService, di.DB](tc.svc, tc.key)

			if tc.wantOK {
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, "postgres://prod", got.DSN)
				return
			}

			require.Error(t, err)

			switch tc.wantErrAs.(type) {
			case di.MissingDependencyError:
				var me di.MissingDependencyError
				require.True(t, errors.As(err, &me))
				assert.Equal(t, tc.key, me.Key)
				assert.Equal(t, `di: dependency "`+string(tc.key)+`" missing`, me.Error())

			case di.WrongTypeDependencyError:
				var we di.WrongTypeDependencyError
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
	var nilSvc *di.Service[di.UserService]
	assert.Nil(t, nilSvc.Clone())

	// covers: else branch where len(s.Deps)==0 -> make(map...)
	empty := &di.Service[di.UserService]{Val: &di.UserService{}, Deps: map[di.DependencyKey]any{}}
	cpEmpty := empty.Clone()
	require.NotNil(t, cpEmpty)
	require.NotNil(t, cpEmpty.Deps)
	assert.Empty(t, cpEmpty.Deps)
	cpEmpty.Deps[di.Key("x")] = "y"
	_, ok := empty.Deps[di.Key("x")]
	assert.False(t, ok)

	// covers: copy deps map but share Val
	key := di.Key("db")
	db := di.Init(func() *di.DB { return &di.DB{DSN: "clone"} })
	user := di.Init(func() *di.UserService { return &di.UserService{} })
	_, err := user.With(di.Injecting(key, db, func(u *di.UserService, d *di.DB) { u.DB = d }))
	require.NoError(t, err)

	cp := user.Clone()
	require.NotNil(t, cp)
	assert.Same(t, user.Val, cp.Val)
	cp.Deps[di.Key("extra")] = "x"
	_, ok = user.Deps[di.Key("extra")]
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
			err:  di.DuplicateKeyError{Key: di.Key("db")},
			want: `di: duplicate dependency key "db"`,
		},
		{
			name: "MissingDependencyError",
			err:  di.MissingDependencyError{Key: di.Key("db")},
			want: `di: dependency "db" missing`,
		},
		{
			name: "WrongTypeDependencyError",
			err:  di.WrongTypeDependencyError{Key: di.Key("logger"), GotType: "*di.Logger"},
			want: `di: dependency "logger" has wrong type (*di.Logger)`,
		},
		{
			name: "NilDependencyServiceError",
			err:  di.NilDependencyServiceError{Key: di.Key("db")},
			want: `di: nil dependency service for key "db"`,
		},
		{
			name: "NilBindError",
			err:  di.NilBindError{Key: di.Key("db")},
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
