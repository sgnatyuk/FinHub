package expense_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/OWNER/finhub/internal/expense"
)

func TestParseTag(t *testing.T) {
	tests := []struct {
		in      string
		want    expense.Tag
		wantErr error
	}{
		{in: "работа", want: "работа"},
		{in: "  #Отпуск-2026 ", want: "отпуск-2026"},
		{in: "WORK", want: "work"},
		{in: "", wantErr: expense.ErrInvalidTag},
		{in: "#", wantErr: expense.ErrInvalidTag},
		{in: "два слова", wantErr: expense.ErrInvalidTag},
		{in: "a_b", wantErr: expense.ErrInvalidTag},
		{in: strings.Repeat("я", 33), wantErr: expense.ErrInvalidTag},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := expense.ParseTag(tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ParseTag(%q) error = %v, want %v", tt.in, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseTag(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseTagsDeduplicates(t *testing.T) {
	got, err := expense.ParseTags([]string{"работа", "#Работа", "такси"})
	if err != nil {
		t.Fatal(err)
	}
	want := []expense.Tag{"работа", "такси"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseTagsLimit(t *testing.T) {
	raw := make([]string, 11)
	for i := range raw {
		raw[i] = "t" + strings.Repeat("x", i)
	}
	if _, err := expense.ParseTags(raw); !errors.Is(err, expense.ErrTooManyTags) {
		t.Errorf("error = %v, want ErrTooManyTags", err)
	}
}
