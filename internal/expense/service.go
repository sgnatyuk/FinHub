package expense

import (
	"context"
	"fmt"
	"time"

	"github.com/OWNER/finhub/internal/category"
	"github.com/OWNER/finhub/internal/money"
	"github.com/OWNER/finhub/internal/user"
)

// Repository — хранилище трат. Интерфейс объявлен здесь, у потребителя,
// а не рядом с Postgres-реализацией.
type Repository interface {
	// Create сохраняет трату и заполняет e.ID.
	Create(ctx context.Context, e *Expense) error
	// List возвращает траты пользователя по фильтру, новые первыми.
	List(ctx context.Context, userID user.ID, f Filter) ([]Expense, error)
	// Delete возвращает ErrNotFound, если траты нет или она чужая.
	Delete(ctx context.Context, userID user.ID, id ID) error
}

// Categories — всё, что сервису трат нужно от категорий. category.Service
// подходит под этот интерфейс без единой строчки «implements».
type Categories interface {
	Get(ctx context.Context, userID user.ID, id category.ID) (*category.Category, error)
	List(ctx context.Context, userID user.ID) ([]category.Category, error)
	Default(ctx context.Context, userID user.ID) (*category.Category, error)
}

// Filter — условия выборки. Нулевое значение поля означает «не фильтровать».
type Filter struct {
	From       time.Time   // включительно
	To         time.Time   // не включительно
	CategoryID category.ID // 0 — любая
	Tags       []Tag       // трата должна иметь все перечисленные теги
}

type Service struct {
	repo       Repository
	categories Categories
	now        func() time.Time
}

func NewService(repo Repository, categories Categories) *Service {
	return &Service{repo: repo, categories: categories, now: time.Now}
}

// RecordInput — входные данные одной структурой: удобнее, чем семь
// позиционных аргументов, и клиентам проще заполнять.
type RecordInput struct {
	UserID     user.ID
	CategoryID category.ID // 0 — категория по умолчанию («Прочее»)
	Amount     money.Money
	Note       string
	Tags       []string  // сырой ввод, нормализуется в ParseTags
	SpentAt    time.Time // нулевое значение — сейчас
}

func (s *Service) Record(ctx context.Context, in RecordInput) (*Expense, error) {
	tags, err := ParseTags(in.Tags)
	if err != nil {
		return nil, err
	}

	cat, err := s.resolveCategory(ctx, in.UserID, in.CategoryID)
	if err != nil {
		return nil, err
	}
	if err := s.checkTagsAgainstCategories(ctx, in.UserID, tags); err != nil {
		return nil, err
	}

	e, err := New(in.UserID, cat.ID, in.Amount, in.Note, tags, in.SpentAt, s.now())
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, e); err != nil {
		return nil, fmt.Errorf("record expense: %w", err)
	}
	return e, nil
}

func (s *Service) List(ctx context.Context, userID user.ID, f Filter) ([]Expense, error) {
	return s.repo.List(ctx, userID, f)
}

func (s *Service) Delete(ctx context.Context, userID user.ID, id ID) error {
	return s.repo.Delete(ctx, userID, id)
}

func (s *Service) resolveCategory(ctx context.Context, userID user.ID, id category.ID) (*category.Category, error) {
	if id == 0 {
		return s.categories.Default(ctx, userID)
	}
	return s.categories.Get(ctx, userID, id)
}

// checkTagsAgainstCategories не даёт завести тег «транспорт», если
// есть категория «Транспорт», — чтобы понятия не смешивались.
func (s *Service) checkTagsAgainstCategories(ctx context.Context, userID user.ID, tags []Tag) error {
	if len(tags) == 0 {
		return nil
	}
	cats, err := s.categories.List(ctx, userID)
	if err != nil {
		return fmt.Errorf("list categories: %w", err)
	}
	for _, t := range tags {
		for _, c := range cats {
			if category.SameName(string(t), c.Name) {
				return fmt.Errorf("%w: %q", ErrTagIsCategory, t)
			}
		}
	}
	return nil
}
