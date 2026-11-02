package expense

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// DateLayout is how dates are written by users and stored in the database.
// Go formats dates by showing the reference time "Mon Jan 2 15:04:05 2006"
// in the shape you want, so "2006-01-02" means YYYY-MM-DD.
const DateLayout = "2006-01-02"

// MonthLayout is the user-facing month format: "2026-11".
const MonthLayout = "2006-01"

var (
	// ErrInvalidExpense is returned (wrapped) when an Expense fails validation.
	ErrInvalidExpense = errors.New("invalid expense")
	// ErrInvalidDate is returned (wrapped) when a date or month string can't be parsed.
	ErrInvalidDate = errors.New("invalid date")
)

// Expense is a single recorded spending event.
type Expense struct {
	ID       int64
	Amount   int64 // whole rupiah, always > 0
	Category string
	Note     string
	Date     time.Time // date only; the time-of-day part is always midnight UTC
}

// Filter narrows down which expenses List returns. The zero value matches everything.
type Filter struct {
	Month    time.Time // first day of the month to match; zero = any month
	Category string    // category name to match; "" = any category
}

// CategoryTotal is one row of a monthly summary.
type CategoryTotal struct {
	Category string
	Total    int64 // whole rupiah
	Count    int   // number of expenses
}

// Validate checks the fields that don't depend on the database.
// Whether the category actually exists is checked by the store.
func (e Expense) Validate() error {
	if e.Amount <= 0 {
		return fmt.Errorf("%w: amount must be greater than zero", ErrInvalidExpense)
	}
	if e.Category == "" {
		return fmt.Errorf("%w: category is required", ErrInvalidExpense)
	}
	if e.Date.IsZero() {
		return fmt.Errorf("%w: date is required", ErrInvalidExpense)
	}
	return nil
}

// MaxCategoryLen is the longest allowed category name, in characters.
const MaxCategoryLen = 30

// ErrInvalidCategory is returned (wrapped) when a category name isn't allowed.
var ErrInvalidCategory = errors.New("invalid category name")

// NormalizeCategory lowercases and collapses whitespace, so "  Food " and
// "food" match, and so do "eating   out" and "eating out".
func NormalizeCategory(s string) string {
	// strings.Fields splits on any run of whitespace and drops empty pieces.
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// ValidateCategoryName checks a (normalized) name for a new category: 1 to
// MaxCategoryLen characters, made of letters, digits, spaces and hyphens.
func ValidateCategoryName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: name is empty", ErrInvalidCategory)
	}
	if utf8.RuneCountInString(name) > MaxCategoryLen {
		return fmt.Errorf("%w: %q is longer than %d characters", ErrInvalidCategory, name, MaxCategoryLen)
	}
	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != ' ' && r != '-' {
			return fmt.Errorf("%w: %q may only contain letters, digits, spaces and hyphens", ErrInvalidCategory, name)
		}
	}
	return nil
}

// ParseDate accepts "YYYY-MM-DD", "today" or "yesterday".
//
// "now" is passed in instead of calling time.Now() inside, so tests can use a
// fixed date and get predictable results.
func ParseDate(s string, now time.Time) (time.Time, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "today":
		return dateOnly(now), nil
	case "yesterday":
		return dateOnly(now).AddDate(0, 0, -1), nil
	}

	// time.Parse returns UTC when the layout has no time zone, which matches dateOnly.
	t, err := time.Parse(DateLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %q (use YYYY-MM-DD, today or yesterday)", ErrInvalidDate, s)
	}
	return t, nil
}

// ParseMonth accepts "YYYY-MM", "current" or "last" and returns the first day
// of that month.
func ParseMonth(s string, now time.Time) (time.Time, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	first := dateOnly(now)
	first = time.Date(first.Year(), first.Month(), 1, 0, 0, 0, 0, time.UTC)

	switch s {
	case "current":
		return first, nil
	case "last":
		// time.Date normalizes out-of-range values: month 0 of 2027 is December 2026.
		return first.AddDate(0, -1, 0), nil
	}

	t, err := time.Parse(MonthLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %q (use YYYY-MM, current or last)", ErrInvalidDate, s)
	}
	return t, nil
}

// dateOnly drops the time of day, keeping the calendar date as seen in t's
// own time zone, and expresses it as midnight UTC.
func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
