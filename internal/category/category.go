// Package category — категории трат: короткий стабильный список
// «на что потрачено». У каждой траты ровно одна категория.
package category

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/OWNER/finhub/internal/user"
)

type ID int64

// DefaultName — категория, в которую попадает трата без явной категории.
const DefaultName = "Прочее"

const maxNameLen = 64

var (
	ErrNotFound      = errors.New("category not found")
	ErrInvalidName   = errors.New("invalid category name")
	ErrDuplicateName = errors.New("category name already exists")
)

type Category struct {
	ID        ID
	UserID    user.ID
	Name      string
	CreatedAt time.Time
}

// New проверяет инварианты и возвращает категорию без ID:
// ID назначит хранилище при сохранении.
func New(userID user.ID, name string, now time.Time) (*Category, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxNameLen {
		return nil, fmt.Errorf("%w: %q", ErrInvalidName, name)
	}
	return &Category{UserID: userID, Name: name, CreatedAt: now}, nil
}

// SameName сравнивает имена без учёта регистра: «Транспорт» и «транспорт»
// для пользователя одно и то же.
func SameName(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
