package di
type DB struct {
	DSN string
}

type Logger struct {
	Level string
}

type BasketService struct {
	DB     *DB
	Logger *Logger
}

type UserService struct {
	DB     *DB
	Logger *Logger
	Basket *BasketService
}
