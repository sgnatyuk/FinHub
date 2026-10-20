package hub

import (
	"context"
	"fmt"
	"time"

	"github.com/OWNER/finhub/internal/user"
)

type Repository interface {
	// Create атомарно сохраняет хаб и его владельца, заполняет h.ID.
	Create(ctx context.Context, h *Hub, owner user.ID, joinedAt time.Time) error
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

// Member — проверка доступа, которой пользуются другие пакеты.
func (s *Service) Member(ctx context.Context, hubID ID, userID user.ID) (*Member, error) {
	return s.repo.Member(ctx, hubID, userID)
}

// AddMember добавляет участника. Делать это может только владелец.
func (s *Service) AddMember(ctx context.Context, actor user.ID, hubID ID, newUser user.ID) error {
	m, err := s.repo.Member(ctx, hubID, actor)
	if err != nil {
		return err
	}
	if !m.IsOwner() {
		return fmt.Errorf("%w: only owner can add members", ErrForbidden)
	}
	return s.repo.AddMember(ctx, Member{HubID: hubID, UserID: newUser, Role: RoleMember, JoinedAt: s.now()})
}

func (s *Service) ListForUser(ctx context.Context, userID user.ID) ([]Hub, error) {
	return s.repo.ListForUser(ctx, userID)
}
