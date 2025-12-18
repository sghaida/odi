package main

import (
	"errors"
	"fmt"
	"log"

	"github.com/sghaida/odi/di"
)

/*
Dependency keys
These are the stable identifiers used to record dependencies in Service.Deps.
Keeping them as constants avoids typos and makes wiring consistent.
*/
const (
	KeyDB     di.DependencyKey = "db"
	KeyLogger di.DependencyKey = "logger"

	// KeyBasketGetter These keys store "interface values" (see below) so that services can depend on interfaces,
	// not concrete types.
	KeyBasketGetter di.DependencyKey = "basketGetter"
	KeyAuthorizer   di.DependencyKey = "authorizer"
)

/*
must unwrap a (*T, error) pair.
This keeps the example readable when wiring services:

	must(svc.WithAll(...))

In real programs you might want to return errors instead of panicking.
*/
func must[T any](v *T, err error) *T {
	if err != nil {
		panic(err)
	}
	return v
}

/*
commonDeps returns the shared injectors for DB + Logger.
This removes the duplicated “inject DB + Logger” fragments across services.
Each caller provides the concrete bind functions for its target type.
*/
func commonDeps[T any](
	db *di.Service[DB],
	logger *di.Service[Logger],
	bindDB func(*T, *DB),
	bindLogger func(*T, *Logger),
) []di.Injector[T] {
	return []di.Injector[T]{
		di.Injecting(KeyDB, db, bindDB),
		di.Injecting(KeyLogger, logger, bindLogger),
	}
}

/*
wireBasket wires BasketService:
  - DB + Logger (common)
  - Authorizer (interface dependency) so Basket can call Pay.Authorize(...) in Checkout
*/
func wireBasket(
	basketSvc *di.Service[BasketService],
	db *di.Service[DB],
	logger *di.Service[Logger],
	authorizer *di.Service[Authorizer],
) error {
	injs := append(
		commonDeps[BasketService](
			db, logger,
			func(s *BasketService, d *DB) { s.DB = d },
			func(s *BasketService, l *Logger) { s.Logger = l },
		),
		di.Injecting(KeyAuthorizer, authorizer, func(s *BasketService, a *Authorizer) {
			s.Pay = *a
		}),
	)
	_, err := basketSvc.WithAll(injs...)
	return err
}

/*
wirePayment wires PaymentService:
  - DB + Logger (common)
  - BasketGetter (interface dependency) so Payment can fetch baskets if basket==nil
*/
func wirePayment(
	paymentSvc *di.Service[PaymentService],
	db *di.Service[DB],
	logger *di.Service[Logger],
	basketGetter *di.Service[BasketGetter],
) error {
	injs := append(
		commonDeps[PaymentService](
			db, logger,
			func(s *PaymentService, d *DB) { s.DB = d },
			func(s *PaymentService, l *Logger) { s.Logger = l },
		),
		di.Injecting(KeyBasketGetter, basketGetter, func(s *PaymentService, bg *BasketGetter) {
			s.Basket = *bg
		}),
	)
	_, err := paymentSvc.WithAll(injs...)
	return err
}

/*
wireUser wires UserService:
  - DB + Logger (common)
  - BasketGetter + Authorizer (interfaces)
*/
func wireUser(
	userSvc *di.Service[UserService],
	db *di.Service[DB],
	logger *di.Service[Logger],
	basketGetter *di.Service[BasketGetter],
	authorizer *di.Service[Authorizer],
) error {
	injs := append(
		commonDeps[UserService](
			db, logger,
			func(s *UserService, d *DB) { s.DB = d },
			func(s *UserService, l *Logger) { s.Logger = l },
		),
		di.Injecting(KeyBasketGetter, basketGetter, func(s *UserService, bg *BasketGetter) {
			s.Basket = *bg
		}),
		di.Injecting(KeyAuthorizer, authorizer, func(s *UserService, a *Authorizer) {
			s.Pay = *a
		}),
	)
	_, err := userSvc.WithAll(injs...)
	return err
}

func main() {
	/*
		1) Init(): create base dependencies
		di.Init constructs a Service[T] by calling ctor and initializing the Deps bag.
	*/
	db := di.Init(func() *DB { return &DB{DSN: "postgres://prod"} })
	logger := di.Init(func() *Logger { return &Logger{Level: "info"} })

	/*
		2) Init(): create concrete services
		We keep services concrete, but inject interfaces into each other to avoid tight coupling.
	*/
	basketSvc := di.Init(func() *BasketService { return &BasketService{} })
	paymentSvc := di.Init(func() *PaymentService { return &PaymentService{} })
	userSvc := di.Init(func() *UserService { return &UserService{} })

	/*
		3) Injecting interfaces (dependency is a *value*, stored in Service.Deps)
		The "interface dependency" is represented as a Service[SomeInterface].
		To make that work, we store a *SomeInterface value* (pointer to interface) in the Service.
	*/
	basketGetter := di.Init(func() *BasketGetter {
		var bg BasketGetter = basketSvc.Value()
		return &bg
	})
	authorizer := di.Init(func() *Authorizer {
		var a Authorizer = paymentSvc.Value()
		return &a
	})

	/*
		4) WithAll(): wire services using reusable wiring functions
	*/
	must(basketSvc, wireBasket(basketSvc, db, logger, authorizer))
	must(paymentSvc, wirePayment(paymentSvc, db, logger, basketGetter))
	must(userSvc, wireUser(userSvc, db, logger, basketGetter, authorizer))

	/*
		5) Value(): use the constructed values
	*/
	userID := "user-123"

	b, err := userSvc.Value().GetUserBasket(userID)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("basket: %+v", b)

	ok, err := userSvc.Value().PlaceOrder(userID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("PlaceOrder authorized: %v\n", ok)

	ok, err = basketSvc.Value().Checkout(userID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Basket Checkout authorized: %v\n", ok)

	/*
		6) Using a concrete service as an interface (no DI helper needed here)
		This demonstrates typical Go usage alongside this DI helper.
	*/
	var ubg UserBasketGetter = userSvc.Value()
	b2, err := ubg.GetUserBasket(userID)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("basket via UserBasketGetter: %+v", b2)

	/*
		7) Has() / GetAny(): introspection of recorded deps
		Has checks whether a key exists in the Deps bag.
		GetAny returns the raw stored value (any).
	*/
	fmt.Printf("User has DB? %v\n", userSvc.Has(KeyDB))
	raw, ok := userSvc.GetAny(KeyDB)
	fmt.Printf("User GetAny(DB) ok=%v, rawType=%T\n", ok, raw)

	/*
		8) GetAs(): typed retrieval with (value, ok)
		This is handy when you want to read back a dep for debugging or tests.
	*/
	gotDB, ok := di.GetAs[UserService, DB](userSvc, KeyDB)
	fmt.Printf("GetAs[DB] ok=%v DSN=%q\n", ok, func() string {
		if gotDB == nil {
			return ""
		}
		return gotDB.DSN
	}())

	/*
		9) TryGetAs(): typed retrieval with typed errors
		TryGetAs returns:
		  - MissingDependencyError if key is not present
		  - WrongTypeDependencyError if key exists but is not *D
	*/
	_, err = di.TryGetAs[UserService, Logger](userSvc, KeyDB) // asking for Logger under DB key
	if err != nil {
		var wt di.WrongTypeDependencyError
		if errors.As(err, &wt) {
			fmt.Printf("TryGetAs wrong-type: key=%q gotType=%s\n", wt.Key, wt.GotType)
		} else {
			fmt.Printf("TryGetAs unexpected error: %v\n", err)
		}
	}

	_, err = di.TryGetAs[UserService, DB](userSvc, "missing-key")
	if err != nil {
		var md di.MissingDependencyError
		if errors.As(err, &md) {
			fmt.Printf("TryGetAs missing: key=%q\n", md.Key)
		}
	}

	/*
		10) MustGetAs(): typed retrieval that panics on missing/wrong-type
		Useful for tests or “cannot happen” wiring assumptions.
	*/
	func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("MustGetAs panicked (as expected): %v\n", r)
			}
		}()
		_ = di.MustGetAs[UserService, Logger](userSvc, KeyDB) // wrong type => panic
	}()

	/*
		11) Clone(): shallow-copy Service + deep-copy Deps map
		Clone shares Val pointer but duplicates the Deps map so further wiring
		or mutation of Deps doesn't affect the original.
	*/
	cloned := userSvc.Clone()
	cloned.Deps["extra"] = "hello"
	_, origHasExtra := userSvc.Deps["extra"]
	fmt.Printf("Clone: original has extra? %v (expected false)\n", origHasExtra)

	/*
		12) Demonstrate error types from Injecting()
		Injecting can return:
		  - ErrNilTarget
		  - NilDependencyServiceError
		  - NilBindError
		  - DuplicateKeyError
	*/
	{
		// 12.(a) DuplicateKeyError: injecting same key twice into same service
		_, err := userSvc.With(di.Injecting(KeyDB, db, func(u *UserService, d *DB) {
			u.DB = d
		}))
		if err != nil {
			var dup di.DuplicateKeyError
			if errors.As(err, &dup) {
				fmt.Printf("Injecting duplicate key: %q\n", dup.Key)
			}
		}

		// 12.(b) ErrNilTarget: applying injector to nil service
		inj := di.Injecting(KeyDB, db, func(u *UserService, d *DB) {})
		if err := inj(nil); err != nil && errors.Is(err, di.ErrNilTarget) {
			fmt.Printf("Injecting nil target => ErrNilTarget\n")
		}

		// 12.c) NilDependencyServiceError: nil dep service
		injNilDep := di.Injecting(KeyDB, (*di.Service[DB])(nil), func(u *UserService, d *DB) {})
		if err := injNilDep(userSvc); err != nil {
			var nde di.NilDependencyServiceError
			if errors.As(err, &nde) {
				fmt.Printf("Injecting nil dep => NilDependencyServiceError key=%q\n", nde.Key)
			}
		}

		// 12.d) NilBindError: nil bind function
		injNilBind := di.Injecting[UserService, DB](KeyDB, db, nil)
		if err := injNilBind(userSvc); err != nil {
			var nb di.NilBindError
			if errors.As(err, &nb) {
				fmt.Printf("Injecting nil bind => NilBindError key=%q\n", nb.Key)
			}
		}
	}
}
