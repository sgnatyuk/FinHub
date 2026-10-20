package stats_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/OWNER/finhub/internal/expense"
	"github.com/OWNER/finhub/internal/hub"
	"github.com/OWNER/finhub/internal/money"
	"github.com/OWNER/finhub/internal/stats"
	"github.com/OWNER/finhub/internal/user"
)

// fakeRepo отдаёт заранее заготовленные строки в «неправильном» порядке
// и запоминает фильтр, с которым его вызвали.
type fakeRepo struct {
	gotFilter expense.Filter
	gotAuthor user.ID
	gotHubs   []hub.ID
}

func (r *fakeRepo) ByCategory(_ context.Context, _ hub.ID, f expense.Filter) ([]stats.CategoryTotal, error) {
	r.gotFilter = f
	return []stats.CategoryTotal{
		{Name: "Кафе", Total: rub(300)},
		{Name: "Транспорт", Total: rub(900)},
		{Name: "Прочее", Total: rub(50)},
	}, nil
}

func (r *fakeRepo) ByTag(context.Context, hub.ID, expense.Filter) ([]stats.TagTotal, error) {
	return nil, nil
}

func (r *fakeRepo) ByAuthor(context.Context, hub.ID, expense.Filter) ([]stats.AuthorTotal, error) {
	return nil, nil
}

func (r *fakeRepo) AuthorByHub(_ context.Context, author user.ID, hubIDs []hub.ID, f expense.Filter) ([]stats.HubTotal, error) {
	r.gotAuthor, r.gotHubs, r.gotFilter = author, hubIDs, f
	return []stats.HubTotal{
		{Name: "Личное", Total: rub(100)},
		{Name: "Семья", Total: rub(500)},
	}, nil
}

func (r *fakeRepo) AuthorByCategoryName(context.Context, user.ID, []hub.ID, expense.Filter) ([]stats.CategoryTotal, error) {
	return nil, nil
}

type fakeHubs map[user.ID][]hub.Hub

func (f fakeHubs) ListForUser(_ context.Context, userID user.ID) ([]hub.Hub, error) {
	return f[userID], nil
}

type onlyMember user.ID

func (m onlyMember) Member(_ context.Context, hubID hub.ID, userID user.ID) (*hub.Member, error) {
	if userID != user.ID(m) {
		return nil, hub.ErrNotMember
	}
	return &hub.Member{HubID: hubID, UserID: userID, Role: hub.RoleMember}, nil
}

func rub(minor int64) money.Money {
	m, _ := money.New(minor, money.RUB)
	return m
}

func TestByCategorySortsAndPassesFilter(t *testing.T) {
	repo := &fakeRepo{}
	svc := stats.NewService(repo, onlyMember(1), fakeHubs{})
	f := stats.Month(2026, time.October, time.UTC)

	got, err := svc.ByCategory(context.Background(), 1, 10, f)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Транспорт", "Кафе", "Прочее"}
	for i, row := range got {
		if row.Name != want[i] {
			t.Errorf("row %d = %q, want %q", i, row.Name, want[i])
		}
	}
	if !repo.gotFilter.From.Equal(f.From) || !repo.gotFilter.To.Equal(f.To) {
		t.Errorf("repo got filter %+v, want %+v", repo.gotFilter, f)
	}
}

func TestByCategoryNotMember(t *testing.T) {
	svc := stats.NewService(&fakeRepo{}, onlyMember(1), fakeHubs{})
	_, err := svc.ByCategory(context.Background(), 2, 10, expense.Filter{})
	if !errors.Is(err, hub.ErrNotMember) {
		t.Errorf("error = %v, want ErrNotMember", err)
	}
}

func TestMonth(t *testing.T) {
	tests := []struct {
		year     int
		month    time.Month
		from, to string
	}{
		{2026, time.October, "2026-10-01", "2026-11-01"},
		{2026, time.December, "2026-12-01", "2027-01-01"},
		{2028, time.February, "2028-02-01", "2028-03-01"},
	}
	for _, tt := range tests {
		f := stats.Month(tt.year, tt.month, time.UTC)
		if got := f.From.Format(time.DateOnly); got != tt.from {
			t.Errorf("Month(%d, %s).From = %s, want %s", tt.year, tt.month, got, tt.from)
		}
		if got := f.To.Format(time.DateOnly); got != tt.to {
			t.Errorf("Month(%d, %s).To = %s, want %s", tt.year, tt.month, got, tt.to)
		}
	}
}

func TestMineByHub(t *testing.T) {
	repo := &fakeRepo{}
	hubs := fakeHubs{1: {{ID: 10, Name: "Личное"}, {ID: 20, Name: "Семья"}}}
	svc := stats.NewService(repo, onlyMember(1), hubs)

	got, err := svc.MineByHub(context.Background(), 1, expense.Filter{CategoryID: 7})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "Семья" {
		t.Errorf("got %+v, want Семья first", got)
	}
	if repo.gotAuthor != 1 || len(repo.gotHubs) != 2 {
		t.Errorf("repo got author %d hubs %v, want author 1 and both hubs", repo.gotAuthor, repo.gotHubs)
	}
	if repo.gotFilter.CategoryID != 0 {
		t.Errorf("CategoryID = %d, want it reset to 0 across hubs", repo.gotFilter.CategoryID)
	}
}

func TestMineWithoutHubs(t *testing.T) {
	svc := stats.NewService(&fakeRepo{}, onlyMember(1), fakeHubs{})
	got, err := svc.MineByHub(context.Background(), 1, expense.Filter{})
	if err != nil || got != nil {
		t.Errorf("got %v, %v; want nil, nil", got, err)
	}
}

var (
	_ stats.Members = (*hub.Service)(nil)
	_ stats.Hubs    = (*hub.Service)(nil)
)
