// Package money представляет денежные суммы без потери точности.
//
// Сумма хранится целым числом в минимальных единицах валюты (копейках),
// поэтому 0.1 + 0.2 здесь всегда ровно 0.3.
package money

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Currency — трёхбуквенный код валюты по ISO 4217, например "RUB".
type Currency string

// RUB — валюта по умолчанию.
const RUB Currency = "RUB"

var (
	ErrInvalidCurrency  = errors.New("money: invalid currency")
	ErrCurrencyMismatch = errors.New("money: currency mismatch")
	ErrInvalidAmount    = errors.New("money: invalid amount")
	ErrOverflow         = errors.New("money: overflow")
)

// Money — неизменяемое значение. Поля неэкспортируемые: получить
// Money с пустой или кривой валютой можно только через нулевое значение.
type Money struct {
	amount   int64
	currency Currency
}

// New создаёт сумму из минимальных единиц: New(12345, RUB) — это 123.45 RUB.
func New(minor int64, cur Currency) (Money, error) {
	if !validCurrency(cur) {
		return Money{}, fmt.Errorf("%w: %q", ErrInvalidCurrency, cur)
	}
	return Money{amount: minor, currency: cur}, nil
}

// Parse разбирает строку вида "123", "123.4" или "123,45".
// Больше двух знаков после запятой — ошибка, а не молчаливое округление.
func Parse(s string, cur Currency) (Money, error) {
	s = strings.TrimSpace(strings.Replace(s, ",", ".", 1))
	whole, frac, hasFrac := strings.Cut(s, ".")
	if whole == "" || strings.HasPrefix(whole, "+") || (hasFrac && (len(frac) == 0 || len(frac) > 2)) {
		return Money{}, fmt.Errorf("%w: %q", ErrInvalidAmount, s)
	}
	neg := strings.HasPrefix(whole, "-")
	units, err := strconv.ParseInt(strings.TrimPrefix(whole, "-"), 10, 64)
	if err != nil {
		return Money{}, fmt.Errorf("%w: %q", ErrInvalidAmount, s)
	}
	var cents int64
	if hasFrac {
		if len(frac) == 1 {
			frac += "0"
		}
		cents, err = strconv.ParseInt(frac, 10, 64)
		if err != nil || strings.HasPrefix(frac, "-") || strings.HasPrefix(frac, "+") {
			return Money{}, fmt.Errorf("%w: %q", ErrInvalidAmount, s)
		}
	}
	if units > (math.MaxInt64-cents)/100 {
		return Money{}, fmt.Errorf("%w: %q", ErrOverflow, s)
	}
	minor := units*100 + cents
	if neg {
		minor = -minor
	}
	return New(minor, cur)
}

// Amount возвращает сумму в минимальных единицах.
func (m Money) Amount() int64 { return m.amount }

// Currency возвращает код валюты.
func (m Money) Currency() Currency { return m.currency }

// IsPositive сообщает, что сумма строго больше нуля.
func (m Money) IsPositive() bool { return m.amount > 0 }

// Add складывает суммы одной валюты.
func (m Money) Add(o Money) (Money, error) {
	if m.currency != o.currency {
		return Money{}, fmt.Errorf("%w: %s and %s", ErrCurrencyMismatch, m.currency, o.currency)
	}
	if (o.amount > 0 && m.amount > math.MaxInt64-o.amount) ||
		(o.amount < 0 && m.amount < math.MinInt64-o.amount) {
		return Money{}, ErrOverflow
	}
	return Money{amount: m.amount + o.amount, currency: m.currency}, nil
}

// String реализует fmt.Stringer: "123.45 RUB".
func (m Money) String() string {
	sign := ""
	a := m.amount
	if a < 0 {
		sign = "-"
	}
	// Делим без смены знака, чтобы не переполниться на MinInt64.
	units, cents := a/100, a%100
	if units < 0 {
		units = -units
	}
	if cents < 0 {
		cents = -cents
	}
	return fmt.Sprintf("%s%d.%02d %s", sign, units, cents, m.currency)
}

func validCurrency(c Currency) bool {
	if len(c) != 3 {
		return false
	}
	for _, r := range c {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}
