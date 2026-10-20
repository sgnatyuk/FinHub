package category

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/OWNER/finhub/internal/hub"
	"github.com/OWNER/finhub/internal/user"
)

// Repository — то, что сервису нужно от хранилища. Реализация
// (Postgres, in-memory в тестах) удовлетворяет интерфейс неявно.
// Права здесь не проверяются: это работа сервиса.
type Repository interface {
	// Create сохраняет категорию и заполняет c.ID.
	// Возвращает ErrDuplicateName, если имя в хабе занято (без учёта регистра).
	Create(ctx context.Context, c *Category) error
	// Get возвращает ErrNotFound, если категории нет в этом хабе.
	Get(ctx context.Context, hubID hub.ID, id ID) (*Category, error)
	// GetByName ищет без учёта регистра; ErrNotFound, если нет.
	GetByName(ctx context.Context, hubID hub.ID, name string) (*Category, error)
	List(ctx context.Context, hubID hub.ID) ([]Category, error)
}

// Members — проверка участия в хабе. Её реализует hub.Service.
type Members interface {
	Member(ctx context.Context, hubID hub.ID, userID user.ID) (*hub.Member, error)
}

type Service struct {
	repo    Repository
	members Members
	now     func() time.Time
}

func NewService(repo Repository, members Members) *Service {
	return &Service{repo: repo, members: members, now: time.Now}
}

// Каждый метод принимает actor — того, кто действует. Вместо Auth::user()
// из глобального состояния он передаётся явно, поэтому сервис легко
// тестировать и вызывать из CLI и бота одинаково.

func (s *Service) Create(ctx context.Context, actor user.ID, hubID hub.ID, name string) (*Category, error) {
	if _, err := s.members.Member(ctx, hubID, actor); err != nil {
		return nil, err
	}
	return s.create(ctx, hubID, name)
}

func (s *Service) Get(ctx context.Context, actor user.ID, hubID hub.ID, id ID) (*Category, error) {
	if _, err := s.members.Member(ctx, hubID, actor); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, hubID, id)
}

func (s *Service) List(ctx context.Context, actor user.ID, hubID hub.ID) ([]Category, error) {
	if _, err := s.members.Member(ctx, hubID, actor); err != nil {
		return nil, err
	}
	return s.repo.List(ctx, hubID)
}

// Default возвращает категорию «Прочее», создавая её при первом обращении.
func (s *Service) Default(ctx context.Context, actor user.ID, hubID hub.ID) (*Category, error) {
	if _, err := s.members.Member(ctx, hubID, actor); err != nil {
		return nil, err
	}
	c, err := s.repo.GetByName(ctx, hubID, DefaultName)
	if err == nil {
		return c, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("get default category: %w", err)
	}
	c, err = s.create(ctx, hubID, DefaultName)
	if errors.Is(err, ErrDuplicateName) {
		// Кто-то создал её параллельно — просто перечитываем.
		return s.repo.GetByName(ctx, hubID, DefaultName)
	}
	return c, err
}

func (s *Service) create(ctx context.Context, hubID hub.ID, name string) (*Category, error) {
	c, err := New(hubID, name, s.now())
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, c); err != nil {
		return nil, fmt.Errorf("create category: %w", err)
	}
	return c, nil
}
