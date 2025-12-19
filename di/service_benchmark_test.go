package di_test

import (
	"testing"

	"github.com/sghaida/odi/di"
)

var (
	dbKey     = di.Key("db")
	loggerKey = di.Key("logger")
)

/*
   Shared helpers (NOT counted in benchmarks)
*/

func newBenchDB() *di.Service[di.DB] {
	return di.Init(func() *di.DB {
		return &di.DB{DSN: "postgres"}
	})
}

func newBenchLogger() *di.Service[di.Logger] {
	return di.Init(func() *di.Logger {
		return &di.Logger{Level: "info"}
	})
}

func newBenchUser() *di.Service[di.UserService] {
	return di.Init(func() *di.UserService {
		return &di.UserService{}
	})
}

/*
   Benchmarks
*/

func BenchmarkInit(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = newBenchUser()
	}
}

func BenchmarkWith_SingleDependency(b *testing.B) {
	db := newBenchDB()
	injDB := di.Injecting(dbKey, db, func(u *di.UserService, d *di.DB) {
		u.DB = d
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		user := newBenchUser()
		_, _ = user.With(injDB)
	}
}

func BenchmarkWithAll_TwoDependencies(b *testing.B) {
	db := newBenchDB()
	logger := newBenchLogger()

	injDB := di.Injecting(dbKey, db, func(u *di.UserService, d *di.DB) {
		u.DB = d
	})
	injLogger := di.Injecting(loggerKey, logger, func(u *di.UserService, l *di.Logger) {
		u.Logger = l
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		user := newBenchUser()
		_, _ = user.WithAll(injDB, injLogger)
	}
}

func BenchmarkHas(b *testing.B) {
	db := newBenchDB()
	user := newBenchUser()

	_, _ = user.With(di.Injecting(dbKey, db, func(u *di.UserService, d *di.DB) {
		u.DB = d
	}))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = user.Has(dbKey)
	}
}

func BenchmarkGetAny(b *testing.B) {
	db := newBenchDB()
	user := newBenchUser()

	_, _ = user.With(di.Injecting(dbKey, db, func(u *di.UserService, d *di.DB) {
		u.DB = d
	}))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = user.GetAny(dbKey)
	}
}

func BenchmarkGetAs(b *testing.B) {
	db := newBenchDB()
	user := newBenchUser()

	_, _ = user.With(di.Injecting(dbKey, db, func(u *di.UserService, d *di.DB) {
		u.DB = d
	}))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = di.GetAs[di.UserService, di.DB](user, dbKey)
	}
}

func BenchmarkTryGetAs_Success(b *testing.B) {
	db := newBenchDB()
	user := newBenchUser()

	_, _ = user.With(di.Injecting(dbKey, db, func(u *di.UserService, d *di.DB) {
		u.DB = d
	}))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = di.TryGetAs[di.UserService, di.DB](user, dbKey)
	}
}

func BenchmarkTryGetAs_Missing(b *testing.B) {
	user := newBenchUser()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = di.TryGetAs[di.UserService, di.DB](user, dbKey)
	}
}

func BenchmarkClone(b *testing.B) {
	db := newBenchDB()
	logger := newBenchLogger()

	user := newBenchUser()
	_, _ = user.WithAll(
		di.Injecting(dbKey, db, func(u *di.UserService, d *di.DB) { u.DB = d }),
		di.Injecting(loggerKey, logger, func(u *di.UserService, l *di.Logger) { u.Logger = l }),
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = user.Clone()
	}
}

func BenchmarkMustGetAs_Success(b *testing.B) {
	db := newBenchDB()
	user := newBenchUser()
	_, _ = user.With(di.Injecting(dbKey, db, func(u *di.UserService, d *di.DB) { u.DB = d }))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = di.MustGetAs[di.UserService, di.DB](user, dbKey)
	}
}

func BenchmarkInjecting_DuplicateKey(b *testing.B) {
	db := newBenchDB()
	user := newBenchUser()
	inj := di.Injecting(dbKey, db, func(u *di.UserService, d *di.DB) { u.DB = d })

	// first time succeeds
	_, _ = user.With(inj)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = user.With(inj) // duplicate path (error)
	}
}

func BenchmarkInjecting_NilTarget(b *testing.B) {
	db := newBenchDB()
	inj := di.Injecting(dbKey, db, func(u *di.UserService, d *di.DB) { u.DB = d })

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = inj(nil) // ErrNilTarget path
	}
}

func BenchmarkInjecting_NilDep(b *testing.B) {
	user := newBenchUser()
	inj := di.Injecting[di.UserService, di.DB](dbKey, nil, func(u *di.UserService, d *di.DB) { u.DB = d })

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = inj(user) // ErrNilDep path
	}
}

func BenchmarkInjecting_NilBind(b *testing.B) {
	db := newBenchDB()
	user := newBenchUser()
	inj := di.Injecting[di.UserService, di.DB](dbKey, db, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = inj(user) // ErrNilBind path
	}
}
