package main

import "fmt"

/*
Interfaces (what we inject)
*/

// BasketGetter exposes the basket read operation.
type BasketGetter interface {
	GetBasket(userID string) (*Basket, error)
}

// Authorizer exposes payment authorization.
type Authorizer interface {
	Authorize(userID string, basket *Basket) (bool, error)
}

// UserBasketGetter exposes the higher-level "user basket" operation.
type UserBasketGetter interface {
	GetUserBasket(userID string) (*Basket, error)
}

//Concrete services

// BasketService depends on DB + Logger, and *also* holds an Authorizer.
// It does NOT call Authorize inside GetBasket, so no recursion.
type BasketService struct {
	DB     *DB
	Logger *Logger
	Pay    Authorizer
}

// GetBasket returns a basket. It does not call Pay.Authorize (important to avoid recursion).
func (s *BasketService) GetBasket(userID string) (*Basket, error) {
	if s.DB == nil || s.Logger == nil {
		return nil, fmt.Errorf("basket: missing DB/Logger wiring")
	}
	return &Basket{
		UserID: userID,
		Items: []BasketItem{
			{SKU: "apple", Qty: 2, Price: 3},
			{SKU: "banana", Qty: 5, Price: 1},
		},
	}, nil
}

// Checkout demonstrates Basket -> Authorizer usage (Basket depends on Payment via interface).
// It calls Pay.Authorize, which will call back into GetBasket via Payment's BasketGetter,
// but GetBasket does not call Authorize, so the call chain terminates.
func (s *BasketService) Checkout(userID string) (bool, error) {
	if s.Pay == nil {
		return false, fmt.Errorf("basket: missing Authorizer wiring")
	}
	b, err := s.GetBasket(userID)
	if err != nil {
		return false, err
	}
	return s.Pay.Authorize(userID, b)
}

// PaymentService depends on DB + Logger and a BasketGetter.
// This is the other side of the mutual dependency (Payment -> Basket via interface).
type PaymentService struct {
	DB     *DB
	Logger *Logger
	Basket BasketGetter
}

// Authorize uses BasketGetter (which is implemented by BasketService).
func (p *PaymentService) Authorize(userID string, basket *Basket) (bool, error) {
	if p.DB == nil || p.Logger == nil {
		return false, fmt.Errorf("payment: missing DB/Logger wiring")
	}
	if p.Basket == nil {
		return false, fmt.Errorf("payment: missing BasketGetter wiring")
	}

	// Optional: if basket not provided, fetch it via BasketGetter.
	if basket == nil {
		var err error
		basket, err = p.Basket.GetBasket(userID)
		if err != nil {
			return false, err
		}
	}

	// Fake rule: authorize if there is at least one item.
	if len(basket.Items) == 0 {
		return false, nil
	}
	return true, nil
}

// UserService depends on DB + Logger and uses the two operations via interfaces.
type UserService struct {
	DB     *DB
	Logger *Logger
	Basket BasketGetter
	Pay    Authorizer
}

// GetUserBasket is the higher-level method (requested as an interface).
func (u *UserService) GetUserBasket(userID string) (*Basket, error) {
	if u.DB == nil || u.Logger == nil {
		return nil, fmt.Errorf("user: missing DB/Logger wiring")
	}
	if u.Basket == nil {
		return nil, fmt.Errorf("user: missing BasketGetter wiring")
	}
	return u.Basket.GetBasket(userID)
}

// PlaceOrder demonstrates UserService orchestrating both:
func (u *UserService) PlaceOrder(userID string) (bool, error) {
	b, err := u.GetUserBasket(userID)
	if err != nil {
		return false, err
	}
	if u.Pay == nil {
		return false, fmt.Errorf("user: missing Authorizer wiring")
	}
	return u.Pay.Authorize(userID, b)
}
