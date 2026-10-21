package category

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sgnatyuk/FinHub/internal/hub"
	"github.com/sgnatyuk/FinHub/internal/user"
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
	// Rename возвращает ErrDuplicateName, если новое имя занято.
	Rename(ctx context.Context, hubID hub.ID, id ID, name string) error
	// Delete удаляет категорию и в той же транзакции переносит
	// её траты в категорию moveTo. Перенос делает хранилище, потому что
	// пакет category не знает про траты (expense импортирует category,
	// обратный импорт дал бы цикл).
	Delete(ctx context.Context, hubID hub.ID, id, moveTo ID) error
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

// Rename переименовывает категорию. Только владелец хаба: список общий.
func (s *Service) Rename(ctx context.Context, actor user.ID, hubID hub.ID, id ID, name string) error {
	if err := s.requireOwner(ctx, actor, hubID); err != nil {
		return err
	}
	name, err := normalizeName(name)
	if err != nil {
		return err
	}
	c, err := s.repo.Get(ctx, hubID, id)
	if err != nil {
		return err
	}
	if c.IsDefault() {
		return ErrDefault
	}
	return s.repo.Rename(ctx, hubID, id, name)
}

// Delete удаляет категорию, а её траты переходят в «Прочее».
// Только владелец хаба; саму «Прочее» удалить нельзя.
func (s *Service) Delete(ctx context.Context, actor user.ID, hubID hub.ID, id ID) error {
	if err := s.requireOwner(ctx, actor, hubID); err != nil {
		return err
	}
	c, err := s.repo.Get(ctx, hubID, id)
	if err != nil {
		return err
	}
	if c.IsDefault() {
		return ErrDefault
	}
	def, err := s.Default(ctx, actor, hubID)
	if err != nil {
		return err
	}
	return s.repo.Delete(ctx, hubID, id, def.ID)
}

func (s *Service) requireOwner(ctx context.Context, actor user.ID, hubID hub.ID) error {
	m, err := s.members.Member(ctx, hubID, actor)
	if err != nil {
		return err
	}
	if !m.IsOwner() {
		return fmt.Errorf("%w: only owner can change categories", hub.ErrForbidden)
	}
	return nil
}

// CreateStarter заводит в хабе стартовый набор категорий. Уже
// существующие имена пропускаются, поэтому повторный вызов безопасен.
//
// Вызывается сразу после создания хаба. Сам пакет hub этого сделать
// не может: category импортирует hub, и обратный импорт дал бы цикл,
// который Go запрещает. Связку «создать хаб + категории» делает
// вызывающий код (регистрация, команда /hub new).
func (s *Service) CreateStarter(ctx context.Context, actor user.ID, hubID hub.ID) ([]Category, error) {
	if _, err := s.members.Member(ctx, hubID, actor); err != nil {
		return nil, err
	}
	created := make([]Category, 0, len(StarterNames))
	for _, name := range StarterNames {
		c, err := s.create(ctx, hubID, name)
		if errors.Is(err, ErrDuplicateName) {
			continue
		}
		if err != nil {
			return nil, err
		}
		created = append(created, *c)
	}
	return created, nil
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
