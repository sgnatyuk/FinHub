// Package expense — траты хаба: запись, просмотр, удаление.
package expense

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/OWNER/finhub/internal/category"
	"github.com/OWNER/finhub/internal/hub"
	"github.com/OWNER/finhub/internal/money"
	"github.com/OWNER/finhub/internal/user"
)

type ID int64

const maxNoteLen = 500

// Expense — обычная структура без методов Save/Find: хранением
// занимается Repository. Сравните с Eloquent-моделью, которая сама ходит в БД.
type Expense struct {
	ID         ID
	HubID      hub.ID
	AuthorID   user.ID // кто внёс трату
	CategoryID category.ID
	Amount     money.Money
	Note       string
	Tags       []Tag
	SpentAt    time.Time
	CreatedAt  time.Time
}

// New проверяет инварианты траты. Проверки, которым нужны данные из
// хранилища (существует ли категория), делает Service.
func New(hubID hub.ID, authorID user.ID, categoryID category.ID, amount money.Money, note string, tags []Tag, spentAt, now time.Time) (*Expense, error) {
	if !amount.IsPositive() {
		return nil, fmt.Errorf("%w: %s", ErrInvalidAmount, amount)
	}
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > maxNoteLen {
		return nil, fmt.Errorf("%w: max %d characters", ErrNoteTooLong, maxNoteLen)
	}
	if len(tags) > maxTagsPerOne {
		return nil, fmt.Errorf("%w: %d tags, max %d", ErrTooManyTags, len(tags), maxTagsPerOne)
	}
	if spentAt.IsZero() {
		spentAt = now
	}
	if spentAt.After(now) {
		return nil, fmt.Errorf("%w: %s", ErrFutureSpending, spentAt.Format(time.DateOnly))
	}
	return &Expense{
		HubID:      hubID,
		AuthorID:   authorID,
		CategoryID: categoryID,
		Amount:     amount,
		Note:       note,
		Tags:       tags,
		SpentAt:    spentAt,
		CreatedAt:  now,
	}, nil
}
