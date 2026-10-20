package expense

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Tag — контекст траты («работа», «отпуск-2026»). Это значение, а не
// сущность: у тега нет ID, два одинаковых тега равны через ==.
type Tag string

const (
	maxTagLen     = 32
	maxTagsPerOne = 10
)

// ParseTag нормализует ввод пользователя: обрезает пробелы и ведущий '#',
// приводит к нижнему регистру. Допустимы буквы, цифры и дефис.
func ParseTag(s string) (Tag, error) {
	s = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(s), "#"))
	if s == "" || utf8.RuneCountInString(s) > maxTagLen {
		return "", fmt.Errorf("%w: %q", ErrInvalidTag, s)
	}
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' {
			return "", fmt.Errorf("%w: %q", ErrInvalidTag, s)
		}
	}
	return Tag(s), nil
}

// ParseTags разбирает список тегов, убирая дубликаты и сохраняя порядок.
func ParseTags(raw []string) ([]Tag, error) {
	tags := make([]Tag, 0, len(raw))
	seen := make(map[Tag]bool, len(raw))
	for _, s := range raw {
		t, err := ParseTag(s)
		if err != nil {
			return nil, err
		}
		if seen[t] {
			continue
		}
		seen[t] = true
		tags = append(tags, t)
	}
	if len(tags) > maxTagsPerOne {
		return nil, fmt.Errorf("%w: %d tags, max %d", ErrTooManyTags, len(tags), maxTagsPerOne)
	}
	return tags, nil
}
