package di_test

import (
	"testing"

	di2 "github.com/sghaida/odi/examples/di"
)

var (
	dbKey     = di2.Key("db")
	loggerKey = di2.Key("logger")
)

/*
   Shared helpers (NOT counted in benchmarks)
*/

func newBenchDB() *di2.Service[di2.DB] {
	return di2.Init(func() *di2.DB {
		return &di2.DB{DSN: "postgres"}
	})
}

func newBenchLogger() *di2.Service[di2.Logger] {
	return di2.Init(func() *di2.Logger {
		return &di2.Logger{Level: "info"}
	})
}

func newBenchUser() *di2.Service[di2.UserService] {
	return di2.Init(func() *di2.UserService {
		return &di2.UserService{}
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
	injDB := di2.Injecting(dbKey, db, func(u *di2.UserService, d *di2.DB) {
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

	injDB := di2.Injecting(dbKey, db, func(u *di2.UserService, d *di2.DB) {
		u.DB = d
	})
	injLogger := di2.Injecting(loggerKey, logger, func(u *di2.UserService, l *di2.Logger) {
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

	_, _ = user.With(di2.Injecting(dbKey, db, func(u *di2.UserService, d *di2.DB) {
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

	_, _ = user.With(di2.Injecting(dbKey, db, func(u *di2.UserService, d *di2.DB) {
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

	_, _ = user.With(di2.Injecting(dbKey, db, func(u *di2.UserService, d *di2.DB) {
		u.DB = d
	}))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = di2.GetAs[di2.UserService, di2.DB](user, dbKey)
	}
}

func BenchmarkTryGetAs_Success(b *testing.B) {
	db := newBenchDB()
	user := newBenchUser()

	_, _ = user.With(di2.Injecting(dbKey, db, func(u *di2.UserService, d *di2.DB) {
		u.DB = d
	}))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = di2.TryGetAs[di2.UserService, di2.DB](user, dbKey)
	}
}

func BenchmarkTryGetAs_Missing(b *testing.B) {
	user := newBenchUser()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = di2.TryGetAs[di2.UserService, di2.DB](user, dbKey)
	}
}

func BenchmarkClone(b *testing.B) {
	db := newBenchDB()
	logger := newBenchLogger()

	user := newBenchUser()
	_, _ = user.WithAll(
		di2.Injecting(dbKey, db, func(u *di2.UserService, d *di2.DB) { u.DB = d }),
		di2.Injecting(loggerKey, logger, func(u *di2.UserService, l *di2.Logger) { u.Logger = l }),
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = user.Clone()
	}
}

func BenchmarkMustGetAs_Success(b *testing.B) {
	db := newBenchDB()
	user := newBenchUser()
	_, _ = user.With(di2.Injecting(dbKey, db, func(u *di2.UserService, d *di2.DB) { u.DB = d }))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = di2.MustGetAs[di2.UserService, di2.DB](user, dbKey)
	}
}

func BenchmarkInjecting_DuplicateKey(b *testing.B) {
	db := newBenchDB()
	user := newBenchUser()
	inj := di2.Injecting(dbKey, db, func(u *di2.UserService, d *di2.DB) { u.DB = d })

	// first time succeeds
	_, _ = user.With(inj)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = user.With(inj) // duplicate path (error)
	}
}

func BenchmarkInjecting_NilTarget(b *testing.B) {
	db := newBenchDB()
	inj := di2.Injecting(dbKey, db, func(u *di2.UserService, d *di2.DB) { u.DB = d })

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = inj(nil) // ErrNilTarget path
	}
}

func BenchmarkInjecting_NilDep(b *testing.B) {
	user := newBenchUser()
	inj := di2.Injecting[di2.UserService, di2.DB](dbKey, nil, func(u *di2.UserService, d *di2.DB) { u.DB = d })

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = inj(user) // ErrNilDep path
	}
}

func BenchmarkInjecting_NilBind(b *testing.B) {
	db := newBenchDB()
	user := newBenchUser()
	inj := di2.Injecting[di2.UserService, di2.DB](dbKey, db, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = inj(user) // ErrNilBind path
	}
}
