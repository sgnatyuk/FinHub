// Package category — категории трат: короткий стабильный список
// «на что потрачено», общий для всех участников хаба.
// У каждой траты ровно одна категория.
package category

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/OWNER/finhub/internal/hub"
)

type ID int64

// DefaultName — категория, в которую попадает трата без явной категории.
const DefaultName = "Прочее"

// StarterNames — стартовый набор, который копируется в новый хаб.
// Это обычные категории: их можно переименовать или удалить.
var StarterNames = []string{"Продукты", "Транспорт", "Жильё", "Кафе", "Здоровье", DefaultName}

const maxNameLen = 64

var (
	ErrNotFound      = errors.New("category not found")
	ErrInvalidName   = errors.New("invalid category name")
	ErrDuplicateName = errors.New("category name already exists")
)

type Category struct {
	ID        ID
	HubID     hub.ID
	Name      string
	CreatedAt time.Time
}

// New проверяет инварианты и возвращает категорию без ID:
// ID назначит хранилище при сохранении.
func New(hubID hub.ID, name string, now time.Time) (*Category, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxNameLen {
		return nil, fmt.Errorf("%w: %q", ErrInvalidName, name)
	}
	return &Category{HubID: hubID, Name: name, CreatedAt: now}, nil
}

// SameName сравнивает имена без учёта регистра: «Транспорт» и «транспорт»
// в одном хабе — одно и то же.
func SameName(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
