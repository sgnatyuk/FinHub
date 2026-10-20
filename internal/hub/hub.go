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

	"github.com/OWNER/finhub/internal/user"
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
)

type Hub struct {
	ID        ID
	Name      string
	CreatedAt time.Time
}

type Member struct {
	HubID    ID
	UserID   user.ID
	Role     Role
	JoinedAt time.Time
}

// IsOwner — маленький метод на значении вместо проверки роли по всему коду.
func (m Member) IsOwner() bool { return m.Role == RoleOwner }

func New(name string, now time.Time) (*Hub, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxNameLen {
		return nil, fmt.Errorf("%w: %q", ErrInvalidName, name)
	}
	return &Hub{Name: name, CreatedAt: now}, nil
}
