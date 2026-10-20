package category_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/OWNER/finhub/internal/category"
	"github.com/OWNER/finhub/internal/hub"
	"github.com/OWNER/finhub/internal/user"
)

// memRepo — рукописный fake вместо мок-фреймворка: настоящая логика
// хранения в 30 строк, тесты проверяют поведение, а не вызовы.
type memRepo struct {
	nextID category.ID
	items  []category.Category
}

func (r *memRepo) Create(_ context.Context, c *category.Category) error {
	for _, it := range r.items {
		if it.HubID == c.HubID && category.SameName(it.Name, c.Name) {
			return category.ErrDuplicateName
		}
	}
	r.nextID++
	c.ID = r.nextID
	r.items = append(r.items, *c)
	return nil
}

func (r *memRepo) Get(_ context.Context, hubID hub.ID, id category.ID) (*category.Category, error) {
	for _, it := range r.items {
		if it.HubID == hubID && it.ID == id {
			return &it, nil
		}
	}
	return nil, category.ErrNotFound
}

func (r *memRepo) GetByName(_ context.Context, hubID hub.ID, name string) (*category.Category, error) {
	for _, it := range r.items {
		if it.HubID == hubID && category.SameName(it.Name, name) {
			return &it, nil
		}
	}
	return nil, category.ErrNotFound
}

func (r *memRepo) List(_ context.Context, hubID hub.ID) ([]category.Category, error) {
	var out []category.Category
	for _, it := range r.items {
		if it.HubID == hubID {
			out = append(out, it)
		}
	}
	return out, nil
}

// members — фиксированный список «кто в каком хабе».
type members map[hub.ID][]user.ID

func (m members) Member(_ context.Context, hubID hub.ID, userID user.ID) (*hub.Member, error) {
	for _, u := range m[hubID] {
		if u == userID {
			return &hub.Member{HubID: hubID, UserID: userID, Role: hub.RoleMember}, nil
		}
	}
	return nil, hub.ErrNotMember
}

const (
	dad user.ID = 1
	mom user.ID = 2

	family   hub.ID = 10
	personal hub.ID = 20
)

func newService() (*category.Service, *memRepo) {
	repo := &memRepo{}
	return category.NewService(repo, members{family: {dad, mom}, personal: {dad}}), repo
}

func TestCreate(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		actor   user.ID
		hub     hub.ID
		input   string
		want    string
		wantErr error
	}{
		{name: "ok", actor: dad, hub: family, input: "Транспорт", want: "Транспорт"},
		{name: "trims spaces", actor: dad, hub: family, input: "  Кафе ", want: "Кафе"},
		{name: "empty", actor: dad, hub: family, input: "   ", wantErr: category.ErrInvalidName},
		{name: "too long", actor: dad, hub: family, input: strings.Repeat("я", 65), wantErr: category.ErrInvalidName},
		{name: "not a member", actor: mom, hub: personal, input: "Кафе", wantErr: hub.ErrNotMember},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := newService()
			c, err := svc.Create(ctx, tt.actor, tt.hub, tt.input)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if err == nil && c.Name != tt.want {
				t.Errorf("name = %q, want %q", c.Name, tt.want)
			}
		})
	}
}

func TestCategoriesAreSharedInHub(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService()
	if _, err := svc.Create(ctx, dad, family, "Транспорт"); err != nil {
		t.Fatal(err)
	}
	// Мама видит категорию папы и не может завести такую же.
	if _, err := svc.Create(ctx, mom, family, "транспорт"); !errors.Is(err, category.ErrDuplicateName) {
		t.Errorf("error = %v, want ErrDuplicateName", err)
	}
	// В другом хабе то же имя допустимо.
	if _, err := svc.Create(ctx, dad, personal, "Транспорт"); err != nil {
		t.Errorf("other hub: %v", err)
	}
}

func TestDefaultIsCreatedOnce(t *testing.T) {
	ctx := context.Background()
	svc, repo := newService()

	first, err := svc.Default(ctx, dad, family)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Default(ctx, mom, family)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || first.Name != category.DefaultName {
		t.Errorf("got %+v and %+v, want the same %q category", first, second, category.DefaultName)
	}
	if len(repo.items) != 1 {
		t.Errorf("stored %d categories, want 1", len(repo.items))
	}
}

func TestGetFromForeignHub(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService()
	c, err := svc.Create(ctx, dad, personal, "Хобби")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, mom, personal, c.ID); !errors.Is(err, hub.ErrNotMember) {
		t.Errorf("error = %v, want ErrNotMember", err)
	}
	// Подставить чужой ID категории в свой хаб тоже не выйдет.
	if _, err := svc.Get(ctx, dad, family, c.ID); !errors.Is(err, category.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

var _ category.Members = (*hub.Service)(nil)
