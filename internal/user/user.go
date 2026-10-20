// Package user описывает владельца данных. Аутентификация (токены CLI,
// привязка Telegram) живёт в других пакетах и сводится к user.ID.
package user

import "time"

// ID — отдельный тип, а не голый int64: компилятор не даст
// перепутать user.ID с category.ID при вызове функции.
type ID int64

type User struct {
	ID        ID
	Name      string
	CreatedAt time.Time
}
