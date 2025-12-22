package di_test

import (
	"testing"

	"github.com/sghaida/odi/di"
)

func benchNew[T any](b *testing.B, ctor func() *T) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = di.New(ctor) // returns di.ServiceV2[T] (value)
	}
}

func benchSingletonDeps() (di.ServiceV2[di.DB], di.ServiceV2[di.Logger]) {
	db := di.New(func() *di.DB { return &di.DB{DSN: "postgres://prod"} })
	logger := di.New(func() *di.Logger { return &di.Logger{Level: "debug"} })
	return db, logger
}

func wireBasket(
	db di.ServiceV2[di.DB],
	logger di.ServiceV2[di.Logger],
) di.ServiceV2[di.BasketService] {
	basket := di.New(func() *di.BasketService { return &di.BasketService{} })
	basket.Val.DB = db.Val
	basket.Val.Logger = logger.Val
	return basket
}

func wireUser(
	db di.ServiceV2[di.DB],
	logger di.ServiceV2[di.Logger],
	basket *di.BasketService,
) di.ServiceV2[di.UserService] {
	user := di.New(func() *di.UserService { return &di.UserService{} })
	user.Val.DB = db.Val
	user.Val.Logger = logger.Val
	user.Val.Basket = basket
	return user
}

func BenchmarkNew_DB(b *testing.B) {
	benchNew(b, func() *di.DB { return &di.DB{DSN: "postgres://prod"} })
}

func BenchmarkNew_Logger(b *testing.B) {
	benchNew(b, func() *di.Logger { return &di.Logger{Level: "info"} })
}

func BenchmarkNew_BasketService(b *testing.B) {
	benchNew(b, func() *di.BasketService { return &di.BasketService{} })
}

func BenchmarkNew_UserService(b *testing.B) {
	benchNew(b, func() *di.UserService { return &di.UserService{} })
}

func BenchmarkWire_BasketService_DB_Logger(b *testing.B) {
	b.ReportAllocs()
	db, logger := benchSingletonDeps()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = wireBasket(db, logger)
	}
}

func BenchmarkWire_Chain_User_Basket_DB_Logger(b *testing.B) {
	b.ReportAllocs()
	db, logger := benchSingletonDeps()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		basket := wireBasket(db, logger)
		_ = wireUser(db, logger, basket.Val)
	}
}
