package category

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/OWNER/finhub/internal/user"
)

// Repository — то, что сервису нужно от хранилища. Реализация
// (Postgres, in-memory в тестах) удовлетворяет интерфейс неявно.
type Repository interface {
	// Create сохраняет категорию и заполняет c.ID.
	// Возвращает ErrDuplicateName, если имя уже занято (без учёта регистра).
	Create(ctx context.Context, c *Category) error
	// Get возвращает ErrNotFound, если категории нет или она чужая.
	Get(ctx context.Context, userID user.ID, id ID) (*Category, error)
	// GetByName ищет без учёта регистра; ErrNotFound, если нет.
	GetByName(ctx context.Context, userID user.ID, name string) (*Category, error)
	List(ctx context.Context, userID user.ID) ([]Category, error)
}

type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

func (s *Service) Create(ctx context.Context, userID user.ID, name string) (*Category, error) {
	c, err := New(userID, name, s.now())
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, c); err != nil {
		return nil, fmt.Errorf("create category: %w", err)
	}
	return c, nil
}

func (s *Service) Get(ctx context.Context, userID user.ID, id ID) (*Category, error) {
	return s.repo.Get(ctx, userID, id)
}

func (s *Service) List(ctx context.Context, userID user.ID) ([]Category, error) {
	return s.repo.List(ctx, userID)
}

// Default возвращает категорию «Прочее», создавая её при первом обращении.
func (s *Service) Default(ctx context.Context, userID user.ID) (*Category, error) {
	c, err := s.repo.GetByName(ctx, userID, DefaultName)
	if err == nil {
		return c, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("get default category: %w", err)
	}
	c, err = s.Create(ctx, userID, DefaultName)
	if errors.Is(err, ErrDuplicateName) {
		// Кто-то создал её параллельно — просто перечитываем.
		return s.repo.GetByName(ctx, userID, DefaultName)
	}
	return c, err
}
