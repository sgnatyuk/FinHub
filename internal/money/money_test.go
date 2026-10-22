package money_test

import (
	"errors"
	"math"
	"testing"

	"github.com/sgnatyuk/FinHub/internal/money"
)

func TestParse(t *testing.T) {
	tests := []struct {
		in      string
		want    int64
		wantErr error
	}{
		{in: "100", want: 10000},
		{in: "123.45", want: 12345},
		{in: "123,45", want: 12345},
		{in: "0.5", want: 50},
		{in: " 7.07 ", want: 707},
		{in: "-3.10", want: -310},
		{in: "", wantErr: money.ErrInvalidAmount},
		{in: "abc", wantErr: money.ErrInvalidAmount},
		{in: "1.234", wantErr: money.ErrInvalidAmount},
		{in: "1.", wantErr: money.ErrInvalidAmount},
		{in: ".5", wantErr: money.ErrInvalidAmount},
		{in: "1.-5", wantErr: money.ErrInvalidAmount},
		{in: "+1", wantErr: money.ErrInvalidAmount},
		{in: "--5", wantErr: money.ErrInvalidAmount},
		{in: "-+5", wantErr: money.ErrInvalidAmount},
		{in: "92233720368547758.08", wantErr: money.ErrOverflow},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := money.Parse(tt.in, money.RUB)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Parse(%q) error = %v, want %v", tt.in, err, tt.wantErr)
			}
			if err == nil && got.Amount() != tt.want {
				t.Errorf("Parse(%q) = %d, want %d", tt.in, got.Amount(), tt.want)
			}
		})
	}
}

func TestNewRejectsBadCurrency(t *testing.T) {
	for _, c := range []money.Currency{"", "rub", "RU", "RUBL", "EUR"} {
		if _, err := money.New(1, c); !errors.Is(err, money.ErrInvalidCurrency) {
			t.Errorf("New(1, %q) error = %v, want ErrInvalidCurrency", c, err)
		}
	}
}

func TestParseCurrency(t *testing.T) {
	for in, want := range map[string]money.Currency{"rub": money.RUB, " USD ": money.USD} {
		got, err := money.ParseCurrency(in)
		if err != nil || got != want {
			t.Errorf("ParseCurrency(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := money.ParseCurrency("btc"); !errors.Is(err, money.ErrInvalidCurrency) {
		t.Errorf("error = %v, want ErrInvalidCurrency", err)
	}
}

func TestAdd(t *testing.T) {
	a := mustNew(t, 150, money.RUB)
	b := mustNew(t, 250, money.RUB)
	sum, err := a.Add(b)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Amount() != 400 {
		t.Errorf("sum = %d, want 400", sum.Amount())
	}

	if _, err := a.Add(mustNew(t, 1, money.USD)); !errors.Is(err, money.ErrCurrencyMismatch) {
		t.Errorf("error = %v, want ErrCurrencyMismatch", err)
	}
	if _, err := mustNew(t, math.MaxInt64, money.RUB).Add(a); !errors.Is(err, money.ErrOverflow) {
		t.Errorf("error = %v, want ErrOverflow", err)
	}
}

func TestString(t *testing.T) {
	tests := map[int64]string{
		12345:         "123.45 RUB",
		5:             "0.05 RUB",
		-5:            "-0.05 RUB",
		-310:          "-3.10 RUB",
		math.MinInt64: "-92233720368547758.08 RUB",
	}
	for in, want := range tests {
		if got := mustNew(t, in, money.RUB).String(); got != want {
			t.Errorf("String(%d) = %q, want %q", in, got, want)
		}
	}
}

func mustNew(t *testing.T, minor int64, c money.Currency) money.Money {
	t.Helper()
	m, err := money.New(minor, c)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
