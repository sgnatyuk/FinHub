package expense_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/OWNER/finhub/internal/category"
	"github.com/OWNER/finhub/internal/expense"
	"github.com/OWNER/finhub/internal/money"
	"github.com/OWNER/finhub/internal/user"
)

type memExpenses struct {
	nextID expense.ID
	items  []expense.Expense
}

func (r *memExpenses) Create(_ context.Context, e *expense.Expense) error {
	r.nextID++
	e.ID = r.nextID
	r.items = append(r.items, *e)
	return nil
}

func (r *memExpenses) List(_ context.Context, userID user.ID, f expense.Filter) ([]expense.Expense, error) {
	var out []expense.Expense
	for _, e := range r.items {
		if e.UserID != userID ||
			(!f.From.IsZero() && e.SpentAt.Before(f.From)) ||
			(!f.To.IsZero() && !e.SpentAt.Before(f.To)) ||
			(f.CategoryID != 0 && e.CategoryID != f.CategoryID) {
			continue
		}
		if !hasAll(e.Tags, f.Tags) {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

func (r *memExpenses) Delete(_ context.Context, userID user.ID, id expense.ID) error {
	for i, e := range r.items {
		if e.UserID == userID && e.ID == id {
			r.items = slices.Delete(r.items, i, i+1)
			return nil
		}
	}
	return expense.ErrNotFound
}

func hasAll(have, want []expense.Tag) bool {
	for _, t := range want {
		if !slices.Contains(have, t) {
			return false
		}
	}
	return true
}

// fakeCategories реализует expense.Categories. Сервис трат не знает,
// что за ним стоит: настоящий category.Service или эта заглушка.
type fakeCategories struct {
	items []category.Category
}

func (f *fakeCategories) Get(_ context.Context, userID user.ID, id category.ID) (*category.Category, error) {
	for _, c := range f.items {
		if c.UserID == userID && c.ID == id {
			return &c, nil
		}
	}
	return nil, category.ErrNotFound
}

func (f *fakeCategories) List(_ context.Context, userID user.ID) ([]category.Category, error) {
	var out []category.Category
	for _, c := range f.items {
		if c.UserID == userID {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeCategories) Default(_ context.Context, userID user.ID) (*category.Category, error) {
	for _, c := range f.items {
		if c.UserID == userID && c.Name == category.DefaultName {
			return &c, nil
		}
	}
	c := category.Category{ID: category.ID(len(f.items) + 1), UserID: userID, Name: category.DefaultName}
	f.items = append(f.items, c)
	return &c, nil
}

const (
	alice     user.ID     = 1
	bob       user.ID     = 2
	transport category.ID = 10
)

func newService() (*expense.Service, *memExpenses) {
	repo := &memExpenses{}
	cats := &fakeCategories{items: []category.Category{
		{ID: transport, UserID: alice, Name: "Транспорт"},
	}}
	return expense.NewService(repo, cats), repo
}

func rub(t *testing.T, minor int64) money.Money {
	t.Helper()
	m, err := money.New(minor, money.RUB)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestRecord(t *testing.T) {
	ctx := context.Background()
	yesterday := time.Now().Add(-24 * time.Hour)

	tests := []struct {
		name    string
		in      expense.RecordInput
		wantErr error
	}{
		{
			name: "ok with tags",
			in:   expense.RecordInput{UserID: alice, CategoryID: transport, Amount: rub(t, 25000), Tags: []string{"работа"}, SpentAt: yesterday},
		},
		{
			name:    "zero amount",
			in:      expense.RecordInput{UserID: alice, CategoryID: transport, Amount: rub(t, 0)},
			wantErr: expense.ErrInvalidAmount,
		},
		{
			name:    "negative amount",
			in:      expense.RecordInput{UserID: alice, CategoryID: transport, Amount: rub(t, -100)},
			wantErr: expense.ErrInvalidAmount,
		},
		{
			name:    "someone else's category",
			in:      expense.RecordInput{UserID: bob, CategoryID: transport, Amount: rub(t, 100)},
			wantErr: category.ErrNotFound,
		},
		{
			name:    "tag equals category name",
			in:      expense.RecordInput{UserID: alice, CategoryID: transport, Amount: rub(t, 100), Tags: []string{"#транспорт"}},
			wantErr: expense.ErrTagIsCategory,
		},
		{
			name:    "invalid tag",
			in:      expense.RecordInput{UserID: alice, CategoryID: transport, Amount: rub(t, 100), Tags: []string{"два слова"}},
			wantErr: expense.ErrInvalidTag,
		},
		{
			name:    "future date",
			in:      expense.RecordInput{UserID: alice, CategoryID: transport, Amount: rub(t, 100), SpentAt: time.Now().Add(48 * time.Hour)},
			wantErr: expense.ErrFutureSpending,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newService()
			e, err := svc.Record(ctx, tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				if len(repo.items) != 0 {
					t.Errorf("stored %d expenses after error, want 0", len(repo.items))
				}
				return
			}
			if e.ID == 0 {
				t.Error("ID was not assigned")
			}
		})
	}
}

func TestRecordDefaults(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService()
	before := time.Now()

	e, err := svc.Record(ctx, expense.RecordInput{UserID: alice, Amount: rub(t, 100)})
	if err != nil {
		t.Fatal(err)
	}
	if e.CategoryID == transport || e.CategoryID == 0 {
		t.Errorf("CategoryID = %d, want the default category", e.CategoryID)
	}
	if e.SpentAt.Before(before) {
		t.Errorf("SpentAt = %v, want now", e.SpentAt)
	}
}

func TestListFiltersByTags(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService()
	record := func(tags ...string) {
		t.Helper()
		_, err := svc.Record(ctx, expense.RecordInput{UserID: alice, CategoryID: transport, Amount: rub(t, 100), Tags: tags})
		if err != nil {
			t.Fatal(err)
		}
	}
	record("работа")
	record("работа", "такси")
	record("отпуск-2026")
	record()

	got, err := svc.List(ctx, alice, expense.Filter{Tags: []expense.Tag{"работа"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("got %d expenses tagged работа, want 2", len(got))
	}
}

func TestDeleteOthersExpense(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService()
	e, err := svc.Record(ctx, expense.RecordInput{UserID: alice, CategoryID: transport, Amount: rub(t, 100)})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, bob, e.ID); !errors.Is(err, expense.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
	if err := svc.Delete(ctx, alice, e.ID); err != nil {
		t.Errorf("owner delete: %v", err)
	}
}

// Проверка на этапе компиляции: настоящий category.Service подходит
// под интерфейс, который объявил пакет expense.
var _ expense.Categories = (*category.Service)(nil)
