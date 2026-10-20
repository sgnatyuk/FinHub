package category_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/OWNER/finhub/internal/category"
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
		if it.UserID == c.UserID && category.SameName(it.Name, c.Name) {
			return category.ErrDuplicateName
		}
	}
	r.nextID++
	c.ID = r.nextID
	r.items = append(r.items, *c)
	return nil
}

func (r *memRepo) Get(_ context.Context, userID user.ID, id category.ID) (*category.Category, error) {
	for _, it := range r.items {
		if it.UserID == userID && it.ID == id {
			return &it, nil
		}
	}
	return nil, category.ErrNotFound
}

func (r *memRepo) GetByName(_ context.Context, userID user.ID, name string) (*category.Category, error) {
	for _, it := range r.items {
		if it.UserID == userID && category.SameName(it.Name, name) {
			return &it, nil
		}
	}
	return nil, category.ErrNotFound
}

func (r *memRepo) List(_ context.Context, userID user.ID) ([]category.Category, error) {
	var out []category.Category
	for _, it := range r.items {
		if it.UserID == userID {
			out = append(out, it)
		}
	}
	return out, nil
}

func TestCreate(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr error
	}{
		{name: "ok", input: "Транспорт", want: "Транспорт"},
		{name: "trims spaces", input: "  Кафе ", want: "Кафе"},
		{name: "empty", input: "   ", wantErr: category.ErrInvalidName},
		{name: "too long", input: strings.Repeat("я", 65), wantErr: category.ErrInvalidName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := category.NewService(&memRepo{})
			c, err := svc.Create(ctx, 1, tt.input)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if err == nil && c.Name != tt.want {
				t.Errorf("name = %q, want %q", c.Name, tt.want)
			}
		})
	}
}

func TestCreateDuplicateIgnoresCase(t *testing.T) {
	ctx := context.Background()
	svc := category.NewService(&memRepo{})
	if _, err := svc.Create(ctx, 1, "Транспорт"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, 1, "транспорт"); !errors.Is(err, category.ErrDuplicateName) {
		t.Errorf("error = %v, want ErrDuplicateName", err)
	}
	// У другого пользователя то же имя допустимо.
	if _, err := svc.Create(ctx, 2, "Транспорт"); err != nil {
		t.Errorf("other user: %v", err)
	}
}

func TestDefaultIsCreatedOnce(t *testing.T) {
	ctx := context.Background()
	repo := &memRepo{}
	svc := category.NewService(repo)

	first, err := svc.Default(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Default(ctx, 1)
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

func TestGetOtherUsersCategory(t *testing.T) {
	ctx := context.Background()
	svc := category.NewService(&memRepo{})
	c, err := svc.Create(ctx, 1, "Кафе")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, 2, c.ID); !errors.Is(err, category.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}
