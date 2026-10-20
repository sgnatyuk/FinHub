package expense_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/sgnatyuk/FinHub/internal/category"
	"github.com/sgnatyuk/FinHub/internal/expense"
	"github.com/sgnatyuk/FinHub/internal/hub"
	"github.com/sgnatyuk/FinHub/internal/money"
	"github.com/sgnatyuk/FinHub/internal/user"
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

func (r *memExpenses) Get(_ context.Context, hubID hub.ID, id expense.ID) (*expense.Expense, error) {
	for _, e := range r.items {
		if e.HubID == hubID && e.ID == id {
			return &e, nil
		}
	}
	return nil, expense.ErrNotFound
}

func (r *memExpenses) List(_ context.Context, hubID hub.ID, f expense.Filter) ([]expense.Expense, error) {
	var out []expense.Expense
	for _, e := range r.items {
		if e.HubID != hubID ||
			(!f.From.IsZero() && e.SpentAt.Before(f.From)) ||
			(!f.To.IsZero() && !e.SpentAt.Before(f.To)) ||
			(f.AuthorID != 0 && e.AuthorID != f.AuthorID) ||
			(f.CategoryID != 0 && e.CategoryID != f.CategoryID) ||
			!hasAll(e.Tags, f.Tags) {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

func (r *memExpenses) Update(_ context.Context, e *expense.Expense) error {
	for i := range r.items {
		if r.items[i].HubID == e.HubID && r.items[i].ID == e.ID {
			r.items[i] = *e
			return nil
		}
	}
	return expense.ErrNotFound
}

func (r *memExpenses) Delete(_ context.Context, hubID hub.ID, id expense.ID) error {
	for i, e := range r.items {
		if e.HubID == hubID && e.ID == id {
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
// Права проверяет сам сервис трат, поэтому здесь actor игнорируется.
type fakeCategories struct {
	items []category.Category
}

func (f *fakeCategories) Get(_ context.Context, _ user.ID, hubID hub.ID, id category.ID) (*category.Category, error) {
	for _, c := range f.items {
		if c.HubID == hubID && c.ID == id {
			return &c, nil
		}
	}
	return nil, category.ErrNotFound
}

func (f *fakeCategories) List(_ context.Context, _ user.ID, hubID hub.ID) ([]category.Category, error) {
	var out []category.Category
	for _, c := range f.items {
		if c.HubID == hubID {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeCategories) Default(_ context.Context, _ user.ID, hubID hub.ID) (*category.Category, error) {
	for _, c := range f.items {
		if c.HubID == hubID && c.Name == category.DefaultName {
			return &c, nil
		}
	}
	c := category.Category{ID: category.ID(100 + len(f.items)), HubID: hubID, Name: category.DefaultName}
	f.items = append(f.items, c)
	return &c, nil
}

// fakeHubs: хаб → пользователь → роль. Все хабы в рублях.
type fakeHubs map[hub.ID]map[user.ID]hub.Role

func (f fakeHubs) Get(ctx context.Context, actor user.ID, hubID hub.ID) (*hub.Hub, error) {
	if _, err := f.Member(ctx, hubID, actor); err != nil {
		return nil, err
	}
	return &hub.Hub{ID: hubID, Currency: money.RUB}, nil
}

func (f fakeHubs) Member(_ context.Context, hubID hub.ID, userID user.ID) (*hub.Member, error) {
	role, ok := f[hubID][userID]
	if !ok {
		return nil, hub.ErrNotMember
	}
	return &hub.Member{HubID: hubID, UserID: userID, Role: role}, nil
}

const (
	dad      user.ID = 1
	mom      user.ID = 2
	stranger user.ID = 3

	family   hub.ID = 10
	personal hub.ID = 20

	transport category.ID = 1 // в хабе family
	hobby     category.ID = 2 // в хабе personal
)

func newService() (*expense.Service, *memExpenses) {
	repo := &memExpenses{}
	cats := &fakeCategories{items: []category.Category{
		{ID: transport, HubID: family, Name: "Транспорт"},
		{ID: hobby, HubID: personal, Name: "Хобби"},
	}}
	hubs := fakeHubs{
		family:   {dad: hub.RoleOwner, mom: hub.RoleMember},
		personal: {dad: hub.RoleOwner},
	}
	return expense.NewService(repo, cats, hubs), repo
}

func rub(t *testing.T, minor int64) money.Money {
	t.Helper()
	m, err := money.New(minor, money.RUB)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func usd(t *testing.T, minor int64) money.Money {
	t.Helper()
	m, err := money.New(minor, money.USD)
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
			in:   expense.RecordInput{Actor: mom, HubID: family, CategoryID: transport, Amount: rub(t, 25000), Tags: []string{"работа"}, SpentAt: yesterday},
		},
		{
			name:    "not a member",
			in:      expense.RecordInput{Actor: stranger, HubID: family, CategoryID: transport, Amount: rub(t, 100)},
			wantErr: hub.ErrNotMember,
		},
		{
			name:    "wrong currency",
			in:      expense.RecordInput{Actor: dad, HubID: family, CategoryID: transport, Amount: usd(t, 100)},
			wantErr: expense.ErrWrongCurrency,
		},
		{
			name:    "zero amount",
			in:      expense.RecordInput{Actor: dad, HubID: family, CategoryID: transport, Amount: rub(t, 0)},
			wantErr: expense.ErrInvalidAmount,
		},
		{
			name:    "negative amount",
			in:      expense.RecordInput{Actor: dad, HubID: family, CategoryID: transport, Amount: rub(t, -100)},
			wantErr: expense.ErrInvalidAmount,
		},
		{
			name:    "category from another hub",
			in:      expense.RecordInput{Actor: dad, HubID: family, CategoryID: hobby, Amount: rub(t, 100)},
			wantErr: category.ErrNotFound,
		},
		{
			name:    "tag equals category name",
			in:      expense.RecordInput{Actor: dad, HubID: family, CategoryID: transport, Amount: rub(t, 100), Tags: []string{"#транспорт"}},
			wantErr: expense.ErrTagIsCategory,
		},
		{
			name:    "invalid tag",
			in:      expense.RecordInput{Actor: dad, HubID: family, CategoryID: transport, Amount: rub(t, 100), Tags: []string{"два слова"}},
			wantErr: expense.ErrInvalidTag,
		},
		{
			name:    "future date",
			in:      expense.RecordInput{Actor: dad, HubID: family, CategoryID: transport, Amount: rub(t, 100), SpentAt: time.Now().Add(48 * time.Hour)},
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
			if e.ID == 0 || e.AuthorID != tt.in.Actor {
				t.Errorf("got ID %d, author %d; want assigned ID and author %d", e.ID, e.AuthorID, tt.in.Actor)
			}
		})
	}
}

func TestRecordDefaults(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService()
	before := time.Now()

	e, err := svc.Record(ctx, expense.RecordInput{Actor: dad, HubID: family, Amount: rub(t, 100)})
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

func TestList(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService()
	record := func(actor user.ID, tags ...string) {
		t.Helper()
		in := expense.RecordInput{Actor: actor, HubID: family, CategoryID: transport, Amount: rub(t, 100), Tags: tags}
		if _, err := svc.Record(ctx, in); err != nil {
			t.Fatal(err)
		}
	}
	record(dad, "работа")
	record(dad, "работа", "такси")
	record(mom, "работа")
	record(mom)

	tests := []struct {
		name   string
		filter expense.Filter
		want   int
	}{
		{name: "all", filter: expense.Filter{}, want: 4},
		{name: "by tag", filter: expense.Filter{Tags: []expense.Tag{"работа"}}, want: 3},
		{name: "by two tags", filter: expense.Filter{Tags: []expense.Tag{"работа", "такси"}}, want: 1},
		{name: "by author", filter: expense.Filter{AuthorID: mom}, want: 2},
		{name: "author and tag", filter: expense.Filter{AuthorID: mom, Tags: []expense.Tag{"работа"}}, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.List(ctx, dad, family, tt.filter)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tt.want {
				t.Errorf("got %d expenses, want %d", len(got), tt.want)
			}
		})
	}

	if _, err := svc.List(ctx, stranger, family, expense.Filter{}); !errors.Is(err, hub.ErrNotMember) {
		t.Errorf("stranger: error = %v, want ErrNotMember", err)
	}
}

func TestDelete(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		author  user.ID
		actor   user.ID
		wantErr error
	}{
		{name: "author deletes own", author: mom, actor: mom},
		{name: "owner deletes member's", author: mom, actor: dad},
		{name: "member cannot delete owner's", author: dad, actor: mom, wantErr: expense.ErrForbidden},
		{name: "stranger", author: dad, actor: stranger, wantErr: hub.ErrNotMember},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newService()
			e, err := svc.Record(ctx, expense.RecordInput{Actor: tt.author, HubID: family, CategoryID: transport, Amount: rub(t, 100)})
			if err != nil {
				t.Fatal(err)
			}
			err = svc.Delete(ctx, tt.actor, family, e.ID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			wantLeft := 0
			if tt.wantErr != nil {
				wantLeft = 1
			}
			if len(repo.items) != wantLeft {
				t.Errorf("%d expenses left, want %d", len(repo.items), wantLeft)
			}
		})
	}
}

// ptr — маленький дженерик-хелпер: &"строка" в Go написать нельзя.
func ptr[T any](v T) *T { return &v }

func TestUpdate(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		actor   user.ID
		in      expense.UpdateInput
		check   func(t *testing.T, e *expense.Expense)
		wantErr error
	}{
		{
			name:  "author changes amount only",
			actor: mom,
			in:    expense.UpdateInput{Amount: ptr(rub(t, 777))},
			check: func(t *testing.T, e *expense.Expense) {
				if e.Amount.Amount() != 777 || e.Note != "такси" || len(e.Tags) != 1 {
					t.Errorf("got %+v, want only amount changed", e)
				}
			},
		},
		{
			name:  "owner clears tags and note",
			actor: dad,
			in:    expense.UpdateInput{Tags: ptr([]string{}), Note: ptr("")},
			check: func(t *testing.T, e *expense.Expense) {
				if len(e.Tags) != 0 || e.Note != "" || e.AuthorID != mom {
					t.Errorf("got %+v, want no tags, empty note, author mom", e)
				}
			},
		},
		{
			name:  "move to default category",
			actor: mom,
			in:    expense.UpdateInput{CategoryID: ptr(category.ID(0))},
			check: func(t *testing.T, e *expense.Expense) {
				if e.CategoryID == transport {
					t.Errorf("CategoryID still transport")
				}
			},
		},
		{name: "stranger", actor: stranger, in: expense.UpdateInput{Note: ptr("x")}, wantErr: hub.ErrNotMember},
		{name: "invalid amount", actor: mom, in: expense.UpdateInput{Amount: ptr(rub(t, 0))}, wantErr: expense.ErrInvalidAmount},
		{name: "wrong currency", actor: mom, in: expense.UpdateInput{Amount: ptr(usd(t, 100))}, wantErr: expense.ErrWrongCurrency},
		{name: "category from another hub", actor: mom, in: expense.UpdateInput{CategoryID: ptr(hobby)}, wantErr: category.ErrNotFound},
		{name: "tag equals category", actor: mom, in: expense.UpdateInput{Tags: ptr([]string{"Транспорт"})}, wantErr: expense.ErrTagIsCategory},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo := newService()
			e, err := svc.Record(ctx, expense.RecordInput{
				Actor: mom, HubID: family, CategoryID: transport,
				Amount: rub(t, 100), Note: "такси", Tags: []string{"работа"},
			})
			if err != nil {
				t.Fatal(err)
			}

			got, err := svc.Update(ctx, tt.actor, family, e.ID, tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				if repo.items[0].Amount.Amount() != 100 {
					t.Errorf("stored expense changed after error")
				}
				return
			}
			tt.check(t, got)
			if got.ID != e.ID || !got.CreatedAt.Equal(e.CreatedAt) {
				t.Errorf("ID/CreatedAt changed: %+v", got)
			}
		})
	}
}

func TestUpdateOthersExpenseForbidden(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService()
	e, err := svc.Record(ctx, expense.RecordInput{Actor: dad, HubID: family, CategoryID: transport, Amount: rub(t, 100)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(ctx, mom, family, e.ID, expense.UpdateInput{Note: ptr("моё")}); !errors.Is(err, expense.ErrForbidden) {
		t.Errorf("error = %v, want ErrForbidden", err)
	}
}

// Проверки на этапе компиляции: настоящие сервисы подходят под
// интерфейсы, которые объявил пакет expense.
var (
	_ expense.Categories = (*category.Service)(nil)
	_ expense.Hubs       = (*hub.Service)(nil)
)
