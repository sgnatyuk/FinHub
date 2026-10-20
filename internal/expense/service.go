package expense

import (
	"context"
	"fmt"
	"time"

	"github.com/OWNER/finhub/internal/category"
	"github.com/OWNER/finhub/internal/hub"
	"github.com/OWNER/finhub/internal/money"
	"github.com/OWNER/finhub/internal/user"
)

// Repository — хранилище трат. Интерфейс объявлен здесь, у потребителя,
// а не рядом с Postgres-реализацией. Права здесь не проверяются.
type Repository interface {
	// Create сохраняет трату и заполняет e.ID.
	Create(ctx context.Context, e *Expense) error
	// Get возвращает ErrNotFound, если траты нет в этом хабе.
	Get(ctx context.Context, hubID hub.ID, id ID) (*Expense, error)
	// List возвращает траты хаба по фильтру, новые первыми.
	List(ctx context.Context, hubID hub.ID, f Filter) ([]Expense, error)
	// Delete возвращает ErrNotFound, если траты нет в этом хабе.
	Delete(ctx context.Context, hubID hub.ID, id ID) error
}

// Categories — всё, что сервису трат нужно от категорий. category.Service
// подходит под этот интерфейс без единой строчки «implements».
type Categories interface {
	Get(ctx context.Context, actor user.ID, hubID hub.ID, id category.ID) (*category.Category, error)
	List(ctx context.Context, actor user.ID, hubID hub.ID) ([]category.Category, error)
	Default(ctx context.Context, actor user.ID, hubID hub.ID) (*category.Category, error)
}

// Members — проверка участия в хабе. Её реализует hub.Service.
type Members interface {
	Member(ctx context.Context, hubID hub.ID, userID user.ID) (*hub.Member, error)
}

// Filter — условия выборки. Нулевое значение поля означает «не фильтровать».
type Filter struct {
	From       time.Time   // включительно
	To         time.Time   // не включительно
	AuthorID   user.ID     // 0 — все участники
	CategoryID category.ID // 0 — любая
	Tags       []Tag       // трата должна иметь все перечисленные теги
}

type Service struct {
	repo       Repository
	categories Categories
	members    Members
	now        func() time.Time
}

func NewService(repo Repository, categories Categories, members Members) *Service {
	return &Service{repo: repo, categories: categories, members: members, now: time.Now}
}

// RecordInput — входные данные одной структурой: удобнее, чем восемь
// позиционных аргументов, и клиентам проще заполнять.
type RecordInput struct {
	Actor      user.ID // автор траты
	HubID      hub.ID
	CategoryID category.ID // 0 — категория по умолчанию («Прочее»)
	Amount     money.Money
	Note       string
	Tags       []string  // сырой ввод, нормализуется в ParseTags
	SpentAt    time.Time // нулевое значение — сейчас
}

func (s *Service) Record(ctx context.Context, in RecordInput) (*Expense, error) {
	if _, err := s.members.Member(ctx, in.HubID, in.Actor); err != nil {
		return nil, err
	}
	tags, err := ParseTags(in.Tags)
	if err != nil {
		return nil, err
	}
	cat, err := s.resolveCategory(ctx, in)
	if err != nil {
		return nil, err
	}
	if err := s.checkTagsAgainstCategories(ctx, in.Actor, in.HubID, tags); err != nil {
		return nil, err
	}

	e, err := New(in.HubID, in.Actor, cat.ID, in.Amount, in.Note, tags, in.SpentAt, s.now())
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, e); err != nil {
		return nil, fmt.Errorf("record expense: %w", err)
	}
	return e, nil
}

func (s *Service) List(ctx context.Context, actor user.ID, hubID hub.ID, f Filter) ([]Expense, error) {
	if _, err := s.members.Member(ctx, hubID, actor); err != nil {
		return nil, err
	}
	return s.repo.List(ctx, hubID, f)
}

// Delete разрешён автору траты и владельцу хаба.
func (s *Service) Delete(ctx context.Context, actor user.ID, hubID hub.ID, id ID) error {
	m, err := s.members.Member(ctx, hubID, actor)
	if err != nil {
		return err
	}
	e, err := s.repo.Get(ctx, hubID, id)
	if err != nil {
		return err
	}
	if e.AuthorID != actor && !m.IsOwner() {
		return ErrForbidden
	}
	return s.repo.Delete(ctx, hubID, id)
}

func (s *Service) resolveCategory(ctx context.Context, in RecordInput) (*category.Category, error) {
	if in.CategoryID == 0 {
		return s.categories.Default(ctx, in.Actor, in.HubID)
	}
	return s.categories.Get(ctx, in.Actor, in.HubID, in.CategoryID)
}

// checkTagsAgainstCategories не даёт завести тег «транспорт», если
// в хабе есть категория «Транспорт», — чтобы понятия не смешивались.
func (s *Service) checkTagsAgainstCategories(ctx context.Context, actor user.ID, hubID hub.ID, tags []Tag) error {
	if len(tags) == 0 {
		return nil
	}
	cats, err := s.categories.List(ctx, actor, hubID)
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
