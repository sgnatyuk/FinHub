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
	moved  map[category.ID]category.ID // удалённая → куда перенесли траты
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

func (r *memRepo) Rename(_ context.Context, hubID hub.ID, id category.ID, name string) error {
	for _, it := range r.items {
		if it.HubID == hubID && it.ID != id && category.SameName(it.Name, name) {
			return category.ErrDuplicateName
		}
	}
	for i := range r.items {
		if r.items[i].HubID == hubID && r.items[i].ID == id {
			r.items[i].Name = name
			return nil
		}
	}
	return category.ErrNotFound
}

func (r *memRepo) Delete(_ context.Context, hubID hub.ID, id, moveTo category.ID) error {
	for i, it := range r.items {
		if it.HubID == hubID && it.ID == id {
			r.items = append(r.items[:i], r.items[i+1:]...)
			if r.moved == nil {
				r.moved = map[category.ID]category.ID{}
			}
			r.moved[id] = moveTo
			return nil
		}
	}
	return category.ErrNotFound
}

// members — фиксированный список «кто в каком хабе с какой ролью».
type members map[hub.ID]map[user.ID]hub.Role

func (m members) Member(_ context.Context, hubID hub.ID, userID user.ID) (*hub.Member, error) {
	role, ok := m[hubID][userID]
	if !ok {
		return nil, hub.ErrNotMember
	}
	return &hub.Member{HubID: hubID, UserID: userID, Role: role}, nil
}

const (
	dad user.ID = 1
	mom user.ID = 2

	family   hub.ID = 10
	personal hub.ID = 20
)

func newService() (*category.Service, *memRepo) {
	repo := &memRepo{}
	return category.NewService(repo, members{
		family:   {dad: hub.RoleOwner, mom: hub.RoleMember},
		personal: {dad: hub.RoleOwner},
	}), repo
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

func TestCreateStarter(t *testing.T) {
	ctx := context.Background()
	svc, repo := newService()
	if _, err := svc.Create(ctx, dad, family, "кафе"); err != nil {
		t.Fatal(err)
	}

	created, err := svc.CreateStarter(ctx, dad, family)
	if err != nil {
		t.Fatal(err)
	}
	if want := len(category.StarterNames) - 1; len(created) != want {
		t.Errorf("created %d categories, want %d (Кафе already exists)", len(created), want)
	}

	// Повторный вызов ничего не добавляет.
	if _, err := svc.CreateStarter(ctx, mom, family); err != nil {
		t.Fatal(err)
	}
	if len(repo.items) != len(category.StarterNames) {
		t.Errorf("stored %d categories, want %d", len(repo.items), len(category.StarterNames))
	}

	if _, err := svc.CreateStarter(ctx, mom, personal); !errors.Is(err, hub.ErrNotMember) {
		t.Errorf("error = %v, want ErrNotMember", err)
	}
}

func TestRename(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService()
	cafe, err := svc.Create(ctx, dad, family, "Кафе")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, dad, family, "Продукты"); err != nil {
		t.Fatal(err)
	}
	def, err := svc.Default(ctx, dad, family)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		actor   user.ID
		id      category.ID
		newName string
		wantErr error
	}{
		{name: "member cannot", actor: mom, id: cafe.ID, newName: "Рестораны", wantErr: hub.ErrForbidden},
		{name: "taken", actor: dad, id: cafe.ID, newName: "продукты", wantErr: category.ErrDuplicateName},
		{name: "default", actor: dad, id: def.ID, newName: "Разное", wantErr: category.ErrDefault},
		{name: "empty", actor: dad, id: cafe.ID, newName: " ", wantErr: category.ErrInvalidName},
		{name: "ok", actor: dad, id: cafe.ID, newName: "Рестораны"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := svc.Rename(ctx, tt.actor, family, tt.id, tt.newName); !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestDeleteMovesToDefault(t *testing.T) {
	ctx := context.Background()
	svc, repo := newService()
	cafe, err := svc.Create(ctx, dad, family, "Кафе")
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Delete(ctx, mom, family, cafe.ID); !errors.Is(err, hub.ErrForbidden) {
		t.Errorf("member: error = %v, want ErrForbidden", err)
	}
	if err := svc.Delete(ctx, dad, family, cafe.ID); err != nil {
		t.Fatal(err)
	}
	def, err := svc.Default(ctx, dad, family)
	if err != nil {
		t.Fatal(err)
	}
	if repo.moved[cafe.ID] != def.ID {
		t.Errorf("expenses moved to %d, want default %d", repo.moved[cafe.ID], def.ID)
	}
	if err := svc.Delete(ctx, dad, family, def.ID); !errors.Is(err, category.ErrDefault) {
		t.Errorf("delete default: error = %v, want ErrDefault", err)
	}
}

var _ category.Members = (*hub.Service)(nil)
