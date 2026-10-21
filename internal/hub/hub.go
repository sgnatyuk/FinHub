// Package hub — общий учёт трат на нескольких людей (личный, семейный).
//
// Человек аутентифицируется как user; хаб — это вопрос прав:
// состоит ли он в хабе и с какой ролью.
package hub

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sgnatyuk/FinHub/internal/money"
	"github.com/sgnatyuk/FinHub/internal/user"
)

type ID int64

// Role — роль участника. Строковый тип с константами — идиоматичная
// замена enum, которого в Go нет.
type Role string

const (
	RoleOwner  Role = "owner"
	RoleMember Role = "member"
)

// PersonalName — имя хаба, который создаётся каждому пользователю.
const PersonalName = "Личное"

const maxNameLen = 64

var (
	ErrNotFound      = errors.New("hub not found")
	ErrInvalidName   = errors.New("invalid hub name")
	ErrNotMember     = errors.New("not a hub member")
	ErrAlreadyMember = errors.New("already a hub member")
	ErrForbidden     = errors.New("action not allowed for this role")
	ErrInvalidTZ     = errors.New("unknown timezone")
)

type Hub struct {
	ID   ID
	Name string
	// Currency — валюта, в которой участники записывают траты хаба.
	Currency money.Currency
	// Timezone — IANA-имя пояса («Europe/Moscow»). По нему считаются
	// границы дня и месяца в статистике; сами времена хранятся в UTC.
	Timezone  string
	CreatedAt time.Time
}

// DefaultTimezone — пояс нового хаба.
const DefaultTimezone = "Europe/Moscow"

// Location возвращает часовой пояс хаба для time.In и stats.Month.
// Бинарнику нужна база поясов: в main стоит импортировать _ "time/tzdata",
// иначе в минимальном Docker-образе LoadLocation вернёт ошибку.
func (h Hub) Location() (*time.Location, error) {
	return time.LoadLocation(h.Timezone)
}

type Member struct {
	HubID    ID
	UserID   user.ID
	Role     Role
	JoinedAt time.Time
}

// IsOwner — маленький метод на значении вместо проверки роли по всему коду.
func (m Member) IsOwner() bool { return m.Role == RoleOwner }

// New создаёт хаб с настройками по умолчанию (RUB, Europe/Moscow);
// поменять их можно через Service.SetCurrency и Service.SetTimezone.
func New(name string, now time.Time) (*Hub, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxNameLen {
		return nil, fmt.Errorf("%w: %q", ErrInvalidName, name)
	}
	return &Hub{Name: name, Currency: money.RUB, Timezone: DefaultTimezone, CreatedAt: now}, nil
}
