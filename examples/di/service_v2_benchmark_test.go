package di_test

import (
	"testing"

	di2 "github.com/sghaida/odi/examples/di"
)

func BenchmarkNew_DB(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = di2.New(func() *di2.DB { return &di2.DB{DSN: "postgres://prod"} })
	}
}

func BenchmarkNew_Logger(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = di2.New(func() *di2.Logger { return &di2.Logger{Level: "info"} })
	}
}

func BenchmarkNew_BasketService(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = di2.New(func() *di2.BasketService { return &di2.BasketService{} })
	}
}

func BenchmarkNew_UserService(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = di2.New(func() *di2.UserService { return &di2.UserService{} })
	}
}

// BasketService wired with DB + Logger (manual assignment).
func BenchmarkWire_BasketService_DB_Logger(b *testing.B) {
	b.ReportAllocs()

	// Create deps once (common DI usage: singletons)
	db := di2.New(func() *di2.DB { return &di2.DB{DSN: "postgres://prod"} })
	logger := di2.New(func() *di2.Logger { return &di2.Logger{Level: "debug"} })

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		basket := di2.New(func() *di2.BasketService { return &di2.BasketService{} })

		// "Wiring" step
		basket.Val.DB = db.Val
		basket.Val.Logger = logger.Val

		_ = basket
	}
}

// Chain wiring: BasketService(DB+Logger) then UserService(DB+Logger+Basket).
func BenchmarkWire_Chain_User_Basket_DB_Logger(b *testing.B) {
	b.ReportAllocs()

	// Deps once
	db := di2.New(func() *di2.DB { return &di2.DB{DSN: "postgres://prod"} })
	logger := di2.New(func() *di2.Logger { return &di2.Logger{Level: "debug"} })

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// BasketService wired with DB+Logger
		basket := di2.New(func() *di2.BasketService { return &di2.BasketService{} })
		basket.Val.DB = db.Val
		basket.Val.Logger = logger.Val

		// UserService wired with DB+Logger + BasketService
		user := di2.New(func() *di2.UserService { return &di2.UserService{} })
		user.Val.DB = db.Val
		user.Val.Logger = logger.Val
		user.Val.Basket = basket.Val

		_ = user
	}
}
