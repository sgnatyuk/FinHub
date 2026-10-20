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
	// Update сохраняет изменённую трату целиком.
	Update(ctx context.Context, e *Expense) error
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

// Hubs — что сервису трат нужно от хабов. Его реализует hub.Service.
type Hubs interface {
	// Get возвращает хаб (с его валютой), если actor в нём состоит.
	Get(ctx context.Context, actor user.ID, hubID hub.ID) (*hub.Hub, error)
	// Member возвращает участника с ролью или hub.ErrNotMember.
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
	hubs       Hubs
	now        func() time.Time
}

func NewService(repo Repository, categories Categories, hubs Hubs) *Service {
	return &Service{repo: repo, categories: categories, hubs: hubs, now: time.Now}
}

// RecordInput — входные данные одной структурой: удобнее, чем восемь
// позиционных аргументов, и клиентам проще заполнять.
type RecordInput struct {
	Actor      user.ID // автор траты
	HubID      hub.ID
	CategoryID category.ID // 0 — категория по умолчанию («Прочее»)
	Amount     money.Money // в валюте хаба
	Note       string
	Tags       []string  // сырой ввод, нормализуется в ParseTags
	SpentAt    time.Time // нулевое значение — сейчас
}

func (s *Service) Record(ctx context.Context, in RecordInput) (*Expense, error) {
	h, err := s.hubs.Get(ctx, in.Actor, in.HubID)
	if err != nil {
		return nil, err
	}
	if in.Amount.Currency() != h.Currency {
		return nil, fmt.Errorf("%w: hub uses %s, got %s", ErrWrongCurrency, h.Currency, in.Amount.Currency())
	}
	tags, err := ParseTags(in.Tags)
	if err != nil {
		return nil, err
	}
	cat, err := s.resolveCategory(ctx, in.Actor, in.HubID, in.CategoryID)
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
	if _, err := s.hubs.Member(ctx, hubID, actor); err != nil {
		return nil, err
	}
	return s.repo.List(ctx, hubID, f)
}

// UpdateInput — частичное изменение. Указатель nil означает «не трогать»,
// поэтому можно отличить «не менять заметку» от «стереть заметку» (""):
// в Laravel для этого есть $request->has(), в Go — указатели.
type UpdateInput struct {
	CategoryID *category.ID // 0 — перенести в «Прочее»
	Amount     *money.Money
	Note       *string
	Tags       *[]string // пустой срез — убрать все теги
	SpentAt    *time.Time
}

// Update разрешён автору траты и владельцу хаба. Автор при этом не меняется.
func (s *Service) Update(ctx context.Context, actor user.ID, hubID hub.ID, id ID, in UpdateInput) (*Expense, error) {
	e, err := s.getModifiable(ctx, actor, hubID, id)
	if err != nil {
		return nil, err
	}

	amount := e.Amount
	if in.Amount != nil {
		h, err := s.hubs.Get(ctx, actor, hubID)
		if err != nil {
			return nil, err
		}
		if in.Amount.Currency() != h.Currency {
			return nil, fmt.Errorf("%w: hub uses %s, got %s", ErrWrongCurrency, h.Currency, in.Amount.Currency())
		}
		amount = *in.Amount
	}

	categoryID := e.CategoryID
	if in.CategoryID != nil {
		cat, err := s.resolveCategory(ctx, actor, hubID, *in.CategoryID)
		if err != nil {
			return nil, err
		}
		categoryID = cat.ID
	}

	tags := e.Tags
	if in.Tags != nil {
		if tags, err = ParseTags(*in.Tags); err != nil {
			return nil, err
		}
		if err := s.checkTagsAgainstCategories(ctx, actor, hubID, tags); err != nil {
			return nil, err
		}
	}

	note, spentAt := e.Note, e.SpentAt
	if in.Note != nil {
		note = *in.Note
	}
	if in.SpentAt != nil {
		spentAt = *in.SpentAt
	}

	// Прогоняем результат через New: все инварианты проверяются в одном месте.
	updated, err := New(hubID, e.AuthorID, categoryID, amount, note, tags, spentAt, s.now())
	if err != nil {
		return nil, err
	}
	updated.ID, updated.CreatedAt = e.ID, e.CreatedAt
	if err := s.repo.Update(ctx, updated); err != nil {
		return nil, fmt.Errorf("update expense: %w", err)
	}
	return updated, nil
}

// Delete разрешён автору траты и владельцу хаба.
func (s *Service) Delete(ctx context.Context, actor user.ID, hubID hub.ID, id ID) error {
	if _, err := s.getModifiable(ctx, actor, hubID, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, hubID, id)
}

// getModifiable возвращает трату, если actor может её менять:
// он автор или владелец хаба.
func (s *Service) getModifiable(ctx context.Context, actor user.ID, hubID hub.ID, id ID) (*Expense, error) {
	m, err := s.hubs.Member(ctx, hubID, actor)
	if err != nil {
		return nil, err
	}
	e, err := s.repo.Get(ctx, hubID, id)
	if err != nil {
		return nil, err
	}
	if e.AuthorID != actor && !m.IsOwner() {
		return nil, ErrForbidden
	}
	return e, nil
}

func (s *Service) resolveCategory(ctx context.Context, actor user.ID, hubID hub.ID, id category.ID) (*category.Category, error) {
	if id == 0 {
		return s.categories.Default(ctx, actor, hubID)
	}
	return s.categories.Get(ctx, actor, hubID, id)
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
