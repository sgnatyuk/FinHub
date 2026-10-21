package hub_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sgnatyuk/FinHub/internal/hub"
	"github.com/sgnatyuk/FinHub/internal/money"
	"github.com/sgnatyuk/FinHub/internal/user"
)

type memRepo struct {
	nextID  hub.ID
	hubs    []hub.Hub
	members []hub.Member
}

func (r *memRepo) Create(_ context.Context, h *hub.Hub, owner user.ID, joinedAt time.Time) error {
	r.nextID++
	h.ID = r.nextID
	r.hubs = append(r.hubs, *h)
	r.members = append(r.members, hub.Member{HubID: h.ID, UserID: owner, Role: hub.RoleOwner, JoinedAt: joinedAt})
	return nil
}

func (r *memRepo) Get(_ context.Context, id hub.ID) (*hub.Hub, error) {
	for _, h := range r.hubs {
		if h.ID == id {
			return &h, nil
		}
	}
	return nil, hub.ErrNotFound
}

func (r *memRepo) SetCurrency(_ context.Context, id hub.ID, cur money.Currency) error {
	for i := range r.hubs {
		if r.hubs[i].ID == id {
			r.hubs[i].Currency = cur
			return nil
		}
	}
	return hub.ErrNotFound
}

func (r *memRepo) SetTimezone(_ context.Context, id hub.ID, tz string) error {
	for i := range r.hubs {
		if r.hubs[i].ID == id {
			r.hubs[i].Timezone = tz
			return nil
		}
	}
	return hub.ErrNotFound
}

func (r *memRepo) Member(_ context.Context, hubID hub.ID, userID user.ID) (*hub.Member, error) {
	for _, m := range r.members {
		if m.HubID == hubID && m.UserID == userID {
			return &m, nil
		}
	}
	return nil, hub.ErrNotMember
}

func (r *memRepo) AddMember(ctx context.Context, m hub.Member) error {
	if _, err := r.Member(ctx, m.HubID, m.UserID); err == nil {
		return hub.ErrAlreadyMember
	}
	r.members = append(r.members, m)
	return nil
}

func (r *memRepo) ListForUser(_ context.Context, userID user.ID) ([]hub.Hub, error) {
	var out []hub.Hub
	for _, m := range r.members {
		if m.UserID != userID {
			continue
		}
		for _, h := range r.hubs {
			if h.ID == m.HubID {
				out = append(out, h)
			}
		}
	}
	return out, nil
}

const (
	dad user.ID = 1
	mom user.ID = 2
	kid user.ID = 3
)

func TestCreateMakesOwner(t *testing.T) {
	ctx := context.Background()
	svc := hub.NewService(&memRepo{})

	h, err := svc.Create(ctx, dad, "Семья")
	if err != nil {
		t.Fatal(err)
	}
	m, err := svc.Member(ctx, h.ID, dad)
	if err != nil {
		t.Fatal(err)
	}
	if !m.IsOwner() {
		t.Errorf("role = %q, want owner", m.Role)
	}
}

func TestCreateInvalidName(t *testing.T) {
	svc := hub.NewService(&memRepo{})
	if _, err := svc.Create(context.Background(), dad, "  "); !errors.Is(err, hub.ErrInvalidName) {
		t.Errorf("error = %v, want ErrInvalidName", err)
	}
}

func TestAddMember(t *testing.T) {
	ctx := context.Background()
	svc := hub.NewService(&memRepo{})
	h, err := svc.Create(ctx, dad, "Семья")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		actor   user.ID
		newUser user.ID
		wantErr error
	}{
		{name: "owner adds mom", actor: dad, newUser: mom},
		{name: "twice", actor: dad, newUser: mom, wantErr: hub.ErrAlreadyMember},
		{name: "member cannot add", actor: mom, newUser: kid, wantErr: hub.ErrForbidden},
		{name: "stranger cannot add", actor: kid, newUser: kid, wantErr: hub.ErrNotMember},
	}
	// Подтесты здесь идут по порядку и опираются на состояние предыдущих.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.AddMember(ctx, tt.actor, h.ID, tt.newUser)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestListForUser(t *testing.T) {
	ctx := context.Background()
	svc := hub.NewService(&memRepo{})
	if _, err := svc.CreatePersonal(ctx, dad); err != nil {
		t.Fatal(err)
	}
	family, err := svc.Create(ctx, dad, "Семья")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.AddMember(ctx, dad, family.ID, mom); err != nil {
		t.Fatal(err)
	}

	dadHubs, _ := svc.ListForUser(ctx, dad)
	momHubs, _ := svc.ListForUser(ctx, mom)
	if len(dadHubs) != 2 || len(momHubs) != 1 {
		t.Errorf("dad has %d hubs, mom has %d; want 2 and 1", len(dadHubs), len(momHubs))
	}
}

func TestSetCurrency(t *testing.T) {
	ctx := context.Background()
	svc := hub.NewService(&memRepo{})
	h, err := svc.Create(ctx, dad, "Семья")
	if err != nil {
		t.Fatal(err)
	}
	if h.Currency != money.RUB {
		t.Fatalf("default currency = %q, want RUB", h.Currency)
	}
	if err := svc.AddMember(ctx, dad, h.ID, mom); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		actor   user.ID
		cur     money.Currency
		wantErr error
	}{
		{name: "member cannot", actor: mom, cur: money.USD, wantErr: hub.ErrForbidden},
		{name: "unsupported", actor: dad, cur: "EUR", wantErr: money.ErrInvalidCurrency},
		{name: "owner switches to USD", actor: dad, cur: money.USD},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := svc.SetCurrency(ctx, tt.actor, h.ID, tt.cur); !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}

	got, err := svc.Get(ctx, mom, h.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Currency != money.USD {
		t.Errorf("currency = %q, want USD", got.Currency)
	}
}

func TestSetTimezone(t *testing.T) {
	ctx := context.Background()
	svc := hub.NewService(&memRepo{})
	h, err := svc.Create(ctx, dad, "Семья")
	if err != nil {
		t.Fatal(err)
	}
	if loc, err := h.Location(); err != nil || loc.String() != hub.DefaultTimezone {
		t.Fatalf("default location = %v, %v; want %s", loc, err, hub.DefaultTimezone)
	}
	if err := svc.AddMember(ctx, dad, h.ID, mom); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		actor   user.ID
		tz      string
		wantErr error
	}{
		{name: "member cannot", actor: mom, tz: "Asia/Tbilisi", wantErr: hub.ErrForbidden},
		{name: "unknown", actor: dad, tz: "Mars/Olympus", wantErr: hub.ErrInvalidTZ},
		{name: "empty", actor: dad, tz: "", wantErr: hub.ErrInvalidTZ},
		{name: "owner sets Tbilisi", actor: dad, tz: "Asia/Tbilisi"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := svc.SetTimezone(ctx, tt.actor, h.ID, tt.tz); !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}

	got, err := svc.Get(ctx, mom, h.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Timezone != "Asia/Tbilisi" {
		t.Errorf("timezone = %q, want Asia/Tbilisi", got.Timezone)
	}
}
