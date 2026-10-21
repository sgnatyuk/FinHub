// Package stats — статистика трат хаба. Здесь нет сущностей,
// только запросы: суммы считает хранилище (GROUP BY в SQL),
// а не цикл по всем тратам в памяти.
package stats

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/sgnatyuk/FinHub/internal/category"
	"github.com/sgnatyuk/FinHub/internal/expense"
	"github.com/sgnatyuk/FinHub/internal/hub"
	"github.com/sgnatyuk/FinHub/internal/money"
	"github.com/sgnatyuk/FinHub/internal/user"
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

type HubTotal struct {
	HubID hub.ID
	Name  string
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

	// Отчёты «мои траты везде»: только траты author в перечисленных хабах.
	AuthorByHub(ctx context.Context, author user.ID, hubIDs []hub.ID, f expense.Filter) ([]HubTotal, error)
	// Категории в разных хабах — разные записи, поэтому здесь группировка
	// по имени без учёта регистра; CategoryID в строках результата равен 0.
	AuthorByCategoryName(ctx context.Context, author user.ID, hubIDs []hub.ID, f expense.Filter) ([]CategoryTotal, error)
}

type Members interface {
	Member(ctx context.Context, hubID hub.ID, userID user.ID) (*hub.Member, error)
}

// Hubs — список хабов пользователя. Его реализует hub.Service.
type Hubs interface {
	ListForUser(ctx context.Context, userID user.ID) ([]hub.Hub, error)
}

type Service struct {
	repo    Repository
	members Members
	hubs    Hubs
}

func NewService(repo Repository, members Members, hubs Hubs) *Service {
	return &Service{repo: repo, members: members, hubs: hubs}
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

// MineByHub — сколько actor потратил в каждом из своих хабов.
// Учитываются только хабы, где он участник сейчас.
func (s *Service) MineByHub(ctx context.Context, actor user.ID, f expense.Filter) ([]HubTotal, error) {
	return mine(ctx, s, actor, f, s.repo.AuthorByHub, func(t HubTotal) money.Money { return t.Total })
}

// MineByCategory — мои траты по всем хабам, сгруппированные по имени
// категории: «Транспорт» из «Личного» и из «Семьи» — одна строка.
func (s *Service) MineByCategory(ctx context.Context, actor user.ID, f expense.Filter) ([]CategoryTotal, error) {
	return mine(ctx, s, actor, f, s.repo.AuthorByCategoryName, func(t CategoryTotal) money.Money { return t.Total })
}

func mine[T any](
	ctx context.Context, s *Service, actor user.ID, f expense.Filter,
	fetch func(context.Context, user.ID, []hub.ID, expense.Filter) ([]T, error),
	total func(T) money.Money,
) ([]T, error) {
	hubs, err := s.hubs.ListForUser(ctx, actor)
	if err != nil {
		return nil, fmt.Errorf("list hubs: %w", err)
	}
	if len(hubs) == 0 {
		return nil, nil
	}
	ids := make([]hub.ID, len(hubs))
	for i, h := range hubs {
		ids[i] = h.ID
	}
	// Категория у каждого хаба своя, фильтр по ID через хабы не имеет смысла.
	f.CategoryID = 0
	rows, err := fetch(ctx, actor, ids, f)
	if err != nil {
		return nil, err
	}
	sortByTotal(rows, total)
	return rows, nil
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
	sortByTotal(rows, total)
	return rows, nil
}

func sortByTotal[T any](rows []T, total func(T) money.Money) {
	slices.SortStableFunc(rows, func(a, b T) int {
		return cmp.Compare(total(b).Amount(), total(a).Amount())
	})
}
