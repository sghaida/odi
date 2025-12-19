package examples

// DB is a fake database handle for the example.
type DB struct {
	DSN string
}

// Logger is a tiny logger for the example.
type Logger struct {
	Level string
}

// BasketItem is a strongly-typed basket item (not []string).
type BasketItem struct {
	SKU   string
	Qty   int
	Price int // "cents" for simplicity
}

type Basket struct {
	UserID string
	Items  []BasketItem
}
