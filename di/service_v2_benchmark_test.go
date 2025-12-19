package di_test

import (
	"testing"

	"github.com/sghaida/odi/di"
)

func BenchmarkNew_DB(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = di.New(func() *di.DB { return &di.DB{DSN: "postgres://prod"} })
	}
}

func BenchmarkNew_Logger(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = di.New(func() *di.Logger { return &di.Logger{Level: "info"} })
	}
}

func BenchmarkNew_BasketService(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = di.New(func() *di.BasketService { return &di.BasketService{} })
	}
}

func BenchmarkNew_UserService(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = di.New(func() *di.UserService { return &di.UserService{} })
	}
}

// BasketService wired with DB + Logger (manual assignment).
func BenchmarkWire_BasketService_DB_Logger(b *testing.B) {
	b.ReportAllocs()

	// Create deps once (common DI usage: singletons)
	db := di.New(func() *di.DB { return &di.DB{DSN: "postgres://prod"} })
	logger := di.New(func() *di.Logger { return &di.Logger{Level: "debug"} })

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		basket := di.New(func() *di.BasketService { return &di.BasketService{} })

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
	db := di.New(func() *di.DB { return &di.DB{DSN: "postgres://prod"} })
	logger := di.New(func() *di.Logger { return &di.Logger{Level: "debug"} })

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// BasketService wired with DB+Logger
		basket := di.New(func() *di.BasketService { return &di.BasketService{} })
		basket.Val.DB = db.Val
		basket.Val.Logger = logger.Val

		// UserService wired with DB+Logger + BasketService
		user := di.New(func() *di.UserService { return &di.UserService{} })
		user.Val.DB = db.Val
		user.Val.Logger = logger.Val
		user.Val.Basket = basket.Val

		_ = user
	}
}

