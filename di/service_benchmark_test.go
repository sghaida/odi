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
	return di.Init(func() *di.DB { return &di.DB{DSN: "postgres"} })
}

func newBenchLogger() *di.Service[di.Logger] {
	return di.Init(func() *di.Logger { return &di.Logger{Level: "info"} })
}

func newBenchUser() *di.Service[di.UserService] {
	return di.Init(func() *di.UserService { return &di.UserService{} })
}

func benchInjDB(db *di.Service[di.DB]) di.Injector[di.UserService] {
	return di.Injecting(dbKey, db, func(u *di.UserService, d *di.DB) { u.DB = d })
}

func benchInjLogger(logger *di.Service[di.Logger]) di.Injector[di.UserService] {
	return di.Injecting(loggerKey, logger, func(u *di.UserService, l *di.Logger) { u.Logger = l })
}

// Pre-injected user for “success path” read benchmarks (Has/Get*).
// Setup happens outside the timer in each benchmark.
func benchUserWithDB() (*di.Service[di.UserService], *di.Service[di.DB]) {
	db := newBenchDB()
	user := newBenchUser()
	_, _ = user.With(benchInjDB(db))
	return user, db
}

func benchLoop(b *testing.B, fn func()) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fn()
	}
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
	injDB := benchInjDB(db)

	benchLoop(b, func() {
		user := newBenchUser()
		_, _ = user.With(injDB)
	})
}

func BenchmarkWithAll_TwoDependencies(b *testing.B) {
	db := newBenchDB()
	logger := newBenchLogger()

	injDB := benchInjDB(db)
	injLogger := benchInjLogger(logger)

	benchLoop(b, func() {
		user := newBenchUser()
		_, _ = user.WithAll(injDB, injLogger)
	})
}

func BenchmarkHas(b *testing.B) {
	user, _ := benchUserWithDB()
	benchLoop(b, func() { _ = user.Has(dbKey) })
}

func BenchmarkGetAny(b *testing.B) {
	user, _ := benchUserWithDB()
	benchLoop(b, func() { _, _ = user.GetAny(dbKey) })
}

func BenchmarkGetAs(b *testing.B) {
	user, _ := benchUserWithDB()
	benchLoop(b, func() { _, _ = di.GetAs[di.UserService, di.DB](user, dbKey) })
}

func BenchmarkTryGetAs_Success(b *testing.B) {
	user, _ := benchUserWithDB()
	benchLoop(b, func() { _, _ = di.TryGetAs[di.UserService, di.DB](user, dbKey) })
}

func BenchmarkTryGetAs_Missing(b *testing.B) {
	user := newBenchUser()
	benchLoop(b, func() { _, _ = di.TryGetAs[di.UserService, di.DB](user, dbKey) })
}

func BenchmarkClone(b *testing.B) {
	db := newBenchDB()
	logger := newBenchLogger()

	user := newBenchUser()
	_, _ = user.WithAll(benchInjDB(db), benchInjLogger(logger))

	benchLoop(b, func() { _ = user.Clone() })
}

func BenchmarkMustGetAs_Success(b *testing.B) {
	db := newBenchDB()
	user := newBenchUser()
	_, _ = user.With(benchInjDB(db))

	benchLoop(b, func() { _ = di.MustGetAs[di.UserService, di.DB](user, dbKey) })
}

func BenchmarkInjecting_DuplicateKey(b *testing.B) {
	db := newBenchDB()
	user := newBenchUser()
	inj := benchInjDB(db)

	// first time succeeds
	_, _ = user.With(inj)

	benchLoop(b, func() { _, _ = user.With(inj) }) // duplicate path (error)
}

func BenchmarkInjecting_NilTarget(b *testing.B) {
	db := newBenchDB()
	inj := benchInjDB(db)

	benchLoop(b, func() { _ = inj(nil) }) // ErrNilTarget path
}

func BenchmarkInjecting_NilDep(b *testing.B) {
	user := newBenchUser()
	inj := di.Injecting[di.UserService, di.DB](dbKey, nil, func(u *di.UserService, d *di.DB) { u.DB = d })

	benchLoop(b, func() { _ = inj(user) }) // ErrNilDep path
}

func BenchmarkInjecting_NilBind(b *testing.B) {
	db := newBenchDB()
	user := newBenchUser()
	inj := di.Injecting[di.UserService, di.DB](dbKey, db, nil)

	benchLoop(b, func() { _ = inj(user) }) // ErrNilBind path
}
