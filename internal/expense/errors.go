package expense

import "errors"

// Sentinel-ошибки: клиенты (CLI, бот) проверяют их через errors.Is
// и решают, что показать пользователю. Аналог своих Exception-классов
// в Laravel, но без throw — ошибка просто возвращается.
var (
	ErrNotFound       = errors.New("expense not found")
	ErrInvalidAmount  = errors.New("amount must be positive")
	ErrNoteTooLong    = errors.New("note is too long")
	ErrInvalidTag     = errors.New("invalid tag")
	ErrTooManyTags    = errors.New("too many tags")
	ErrTagIsCategory  = errors.New("tag duplicates a category name")
	ErrFutureSpending = errors.New("spending date is in the future")
)
