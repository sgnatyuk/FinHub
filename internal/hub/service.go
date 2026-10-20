package hub

import (
	"context"
	"fmt"
	"time"

	"github.com/sgnatyuk/FinHub/internal/money"
	"github.com/sgnatyuk/FinHub/internal/user"
)

type Repository interface {
	// Create атомарно сохраняет хаб и его владельца, заполняет h.ID.
	Create(ctx context.Context, h *Hub, owner user.ID, joinedAt time.Time) error
	// Get возвращает ErrNotFound, если хаба нет.
	Get(ctx context.Context, id ID) (*Hub, error)
	SetCurrency(ctx context.Context, id ID, cur money.Currency) error
	SetTimezone(ctx context.Context, id ID, tz string) error
	// Member возвращает ErrNotMember, если пользователь не состоит в хабе.
	Member(ctx context.Context, hubID ID, userID user.ID) (*Member, error)
	// AddMember возвращает ErrAlreadyMember при повторном добавлении.
	AddMember(ctx context.Context, m Member) error
	ListForUser(ctx context.Context, userID user.ID) ([]Hub, error)
}

type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// Create создаёт хаб, где owner — владелец.
func (s *Service) Create(ctx context.Context, owner user.ID, name string) (*Hub, error) {
	now := s.now()
	h, err := New(name, now)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, h, owner, now); err != nil {
		return nil, fmt.Errorf("create hub: %w", err)
	}
	return h, nil
}

// CreatePersonal вызывается при регистрации пользователя.
func (s *Service) CreatePersonal(ctx context.Context, owner user.ID) (*Hub, error) {
	return s.Create(ctx, owner, PersonalName)
}

// Get возвращает хаб, если actor в нём состоит.
func (s *Service) Get(ctx context.Context, actor user.ID, hubID ID) (*Hub, error) {
	if _, err := s.repo.Member(ctx, hubID, actor); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, hubID)
}

// SetCurrency меняет валюту хаба (команда /config). Только владелец.
// Старые траты остаются в прежней валюте, статистика считает валюты раздельно.
func (s *Service) SetCurrency(ctx context.Context, actor user.ID, hubID ID, cur money.Currency) error {
	if err := s.requireOwner(ctx, actor, hubID, "change currency"); err != nil {
		return err
	}
	// Проверяем через money.New, чтобы не дублировать список валют.
	if _, err := money.New(0, cur); err != nil {
		return err
	}
	return s.repo.SetCurrency(ctx, hubID, cur)
}

// SetTimezone меняет часовой пояс хаба (команда /config). Только владелец.
func (s *Service) SetTimezone(ctx context.Context, actor user.ID, hubID ID, tz string) error {
	if err := s.requireOwner(ctx, actor, hubID, "change timezone"); err != nil {
		return err
	}
	// "" и "Local" LoadLocation принимает, но для хаба они бессмысленны.
	if tz == "" || tz == "Local" {
		return fmt.Errorf("%w: %q", ErrInvalidTZ, tz)
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return fmt.Errorf("%w: %q", ErrInvalidTZ, tz)
	}
	return s.repo.SetTimezone(ctx, hubID, tz)
}

func (s *Service) requireOwner(ctx context.Context, actor user.ID, hubID ID, action string) error {
	m, err := s.repo.Member(ctx, hubID, actor)
	if err != nil {
		return err
	}
	if !m.IsOwner() {
		return fmt.Errorf("%w: only owner can %s", ErrForbidden, action)
	}
	return nil
}

// Member — проверка доступа, которой пользуются другие пакеты.
func (s *Service) Member(ctx context.Context, hubID ID, userID user.ID) (*Member, error) {
	return s.repo.Member(ctx, hubID, userID)
}

// AddMember добавляет участника. Делать это может только владелец.
func (s *Service) AddMember(ctx context.Context, actor user.ID, hubID ID, newUser user.ID) error {
	if err := s.requireOwner(ctx, actor, hubID, "add members"); err != nil {
		return err
	}
	return s.repo.AddMember(ctx, Member{HubID: hubID, UserID: newUser, Role: RoleMember, JoinedAt: s.now()})
}

func (s *Service) ListForUser(ctx context.Context, userID user.ID) ([]Hub, error) {
	return s.repo.ListForUser(ctx, userID)
}
