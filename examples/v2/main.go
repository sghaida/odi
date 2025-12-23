// Package v2 contains the runnable example(s) for the v2 approach.
//
// v2 favors simple constructors and explicit manual wiring without any generator,
// keeping the model minimal and easy to understand.
package main

import (
	"fmt"
	"log"

	"github.com/sghaida/odi/di"
	"github.com/sghaida/odi/examples"
)

// main demonstrates DI v2 (ServiceV2) usage:
//
// - Create "service containers" via di.New(...) which simply holds Val *T.
// - Wire dependencies manually by assigning fields on .Val.
// - Use interfaces for the mutual dependency (Basket <-> Payment) to avoid hard coupling.
func main() {
	// --- Base dependencies (usually singletons) ---

	db := di.New(func() *examples.DB { return &examples.DB{DSN: "postgres://prod"} })
	logger := di.New(func() *examples.Logger { return &examples.Logger{Level: "info"} })

	// --- Construct concrete services (empty structs first) ---

	basketSvc := di.New(func() *examples.BasketService { return &examples.BasketService{} })
	paymentSvc := di.New(func() *examples.PaymentService { return &examples.PaymentService{} })
	userSvc := di.New(func() *examples.UserService { return &examples.UserService{} })

	// --- Create interface views (to break concrete cycles) ---
	//
	// BasketService implements BasketGetter.
	// PaymentService implements Authorizer.
	var basketGetter examples.BasketGetter = basketSvc.Val
	var authorizer examples.Authorizer = paymentSvc.Val

	// --- Wire BasketService (DB + Logger + Authorizer) ---

	basketSvc.Val.DB = db.Val
	basketSvc.Val.Logger = logger.Val
	basketSvc.Val.Pay = authorizer

	// --- Wire PaymentService (DB + Logger + BasketGetter) ---

	paymentSvc.Val.DB = db.Val
	paymentSvc.Val.Logger = logger.Val
	paymentSvc.Val.Basket = basketGetter

	// --- Wire UserService (DB + Logger + BasketGetter + Authorizer) ---

	userSvc.Val.DB = db.Val
	userSvc.Val.Logger = logger.Val
	userSvc.Val.Basket = basketGetter
	userSvc.Val.Pay = authorizer

	// --- Demo ---

	userID := "user-123"

	b, err := userSvc.Val.GetUserBasket(userID)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("basket: %+v", b)

	ok, err := userSvc.Val.PlaceOrder(userID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("PlaceOrder authorized: %v\n", ok)

	ok, err = basketSvc.Val.Checkout(userID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Basket Checkout authorized: %v\n", ok)

	// Also show using the higher-level interface if you want:
	var ubg examples.UserBasketGetter = userSvc.Val
	b2, err := ubg.GetUserBasket(userID)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("basket (via UserBasketGetter): %+v", b2)
}
