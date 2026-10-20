// Package stats — статистика трат хаба. Здесь нет сущностей,
// только запросы: суммы считает хранилище (GROUP BY в SQL),
// а не цикл по всем тратам в памяти.
package stats

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/OWNER/finhub/internal/category"
	"github.com/OWNER/finhub/internal/expense"
	"github.com/OWNER/finhub/internal/hub"
	"github.com/OWNER/finhub/internal/money"
	"github.com/OWNER/finhub/internal/user"
)

// Суммы разных валют не складываются, поэтому каждая строка
// результата относится к одной валюте (её несёт Total).

type CategoryTotal struct {
	CategoryID category.ID
	Name       string
	Total      money.Money
	Count      int
}

type TagTotal struct {
	Tag   expense.Tag
	Total money.Money
	Count int
}

type AuthorTotal struct {
	AuthorID user.ID
	Total    money.Money
	Count    int
}

// Repository — агрегирующие запросы. Фильтр тот же, что у списка трат:
// «сколько на транспорт по тегу работа за октябрь» — одна структура.
type Repository interface {
	ByCategory(ctx context.Context, hubID hub.ID, f expense.Filter) ([]CategoryTotal, error)
	ByTag(ctx context.Context, hubID hub.ID, f expense.Filter) ([]TagTotal, error)
	ByAuthor(ctx context.Context, hubID hub.ID, f expense.Filter) ([]AuthorTotal, error)
}

type Members interface {
	Member(ctx context.Context, hubID hub.ID, userID user.ID) (*hub.Member, error)
}

type Service struct {
	repo    Repository
	members Members
}

func NewService(repo Repository, members Members) *Service {
	return &Service{repo: repo, members: members}
}

// Month возвращает фильтр на календарный месяц в часовом поясе loc.
func Month(year int, month time.Month, loc *time.Location) expense.Filter {
	from := time.Date(year, month, 1, 0, 0, 0, 0, loc)
	return expense.Filter{From: from, To: from.AddDate(0, 1, 0)}
}

// ByCategory — «куда ушли деньги»: суммы по категориям складываются в итог.
func (s *Service) ByCategory(ctx context.Context, actor user.ID, hubID hub.ID, f expense.Filter) ([]CategoryTotal, error) {
	return query(ctx, s, actor, hubID, f, s.repo.ByCategory, func(t CategoryTotal) money.Money { return t.Total })
}

// ByTag — срезы по контексту. Трата с двумя тегами попадёт в обе строки,
// поэтому сумма по тегам не равна итогу.
func (s *Service) ByTag(ctx context.Context, actor user.ID, hubID hub.ID, f expense.Filter) ([]TagTotal, error) {
	return query(ctx, s, actor, hubID, f, s.repo.ByTag, func(t TagTotal) money.Money { return t.Total })
}

// ByAuthor — кто сколько потратил в хабе.
func (s *Service) ByAuthor(ctx context.Context, actor user.ID, hubID hub.ID, f expense.Filter) ([]AuthorTotal, error) {
	return query(ctx, s, actor, hubID, f, s.repo.ByAuthor, func(t AuthorTotal) money.Money { return t.Total })
}

// query — общий шаг для всех отчётов: проверить доступ, получить строки,
// отсортировать по убыванию суммы. Дженерик избавляет от трёх копий
// одного и того же кода.
func query[T any](
	ctx context.Context, s *Service, actor user.ID, hubID hub.ID, f expense.Filter,
	fetch func(context.Context, hub.ID, expense.Filter) ([]T, error),
	total func(T) money.Money,
) ([]T, error) {
	if _, err := s.members.Member(ctx, hubID, actor); err != nil {
		return nil, err
	}
	rows, err := fetch(ctx, hubID, f)
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(rows, func(a, b T) int {
		return cmp.Compare(total(b).Amount(), total(a).Amount())
	})
	return rows, nil
}
