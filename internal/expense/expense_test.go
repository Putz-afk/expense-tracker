package expense

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// A fixed "now" late in the evening, to prove the time of day is ignored.
var testNow = time.Date(2026, time.November, 2, 23, 30, 0, 0, time.UTC)

func ymd(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestExpenseValidate(t *testing.T) {
	valid := Expense{Amount: 25000, Category: "food", Date: ymd(2026, 11, 2)}

	tests := []struct {
		name    string
		mutate  func(e *Expense)
		wantErr bool
	}{
		{"valid", func(e *Expense) {}, false},
		{"valid with note", func(e *Expense) { e.Note = "lunch" }, false},
		{"zero amount", func(e *Expense) { e.Amount = 0 }, true},
		{"negative amount", func(e *Expense) { e.Amount = -1 }, true},
		{"empty category", func(e *Expense) { e.Category = "" }, true},
		{"zero date", func(e *Expense) { e.Date = time.Time{} }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := valid // structs are copied on assignment, so valid stays untouched
			tt.mutate(&e)
			err := e.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, ErrInvalidExpense) {
				t.Errorf("error %v does not wrap ErrInvalidExpense", err)
			}
		})
	}
}

func TestNormalizeCategory(t *testing.T) {
	tests := []struct{ in, want string }{
		{"food", "food"},
		{"  Food ", "food"},
		{"TRANSPORT", "transport"},
		{"Eating   Out", "eating out"},
		{"\teating\nout ", "eating out"},
		{"", ""},
		{"   ", ""},
	}
	for _, tt := range tests {
		if got := NormalizeCategory(tt.in); got != tt.want {
			t.Errorf("NormalizeCategory(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestValidateCategoryName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"simple", "groceries", false},
		{"with space", "eating out", false},
		{"with hyphen", "e-books", false},
		{"with digits", "gym 2026", false},
		{"non-ascii letters", "kopi susu é", false},
		{"max length", strings.Repeat("a", MaxCategoryLen), false},
		{"empty", "", true},
		{"too long", strings.Repeat("a", MaxCategoryLen+1), true},
		{"punctuation", "food!", true},
		{"comma", "a,b", true},
		{"quote", "o'brien", true},
		{"slash", "a/b", true},
		{"tab", "a\tb", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCategoryName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateCategoryName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, ErrInvalidCategory) {
				t.Errorf("error %v does not wrap ErrInvalidCategory", err)
			}
		})
	}
}

func TestParseDate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Time
		wantErr bool
	}{
		{"explicit date", "2026-10-15", ymd(2026, 10, 15), false},
		{"today", "today", ymd(2026, 11, 2), false},
		{"today mixed case", " Today ", ymd(2026, 11, 2), false},
		{"yesterday", "yesterday", ymd(2026, 11, 1), false},
		{"leap day", "2028-02-29", ymd(2028, 2, 29), false},
		{"empty", "", time.Time{}, true},
		{"wrong order", "15-10-2026", time.Time{}, true},
		{"month only", "2026-10", time.Time{}, true},
		{"impossible day", "2026-02-30", time.Time{}, true},
		{"not a leap year", "2027-02-29", time.Time{}, true},
		{"words", "tomorrow", time.Time{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDate(tt.input, testNow)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseDate(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, ErrInvalidDate) {
				t.Errorf("error %v does not wrap ErrInvalidDate", err)
			}
			if !got.Equal(tt.want) {
				t.Errorf("ParseDate(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseDateYesterdayAcrossMonthAndYear(t *testing.T) {
	newYear := time.Date(2027, time.January, 1, 8, 0, 0, 0, time.UTC)
	got, err := ParseDate("yesterday", newYear)
	if err != nil {
		t.Fatal(err)
	}
	if want := ymd(2026, 12, 31); !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseMonth(t *testing.T) {
	january := time.Date(2027, time.January, 15, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		input   string
		now     time.Time
		want    time.Time
		wantErr bool
	}{
		{"explicit", "2026-10", testNow, ymd(2026, 10, 1), false},
		{"current", "current", testNow, ymd(2026, 11, 1), false},
		{"last", "last", testNow, ymd(2026, 10, 1), false},
		{"last in January wraps to December", "last", january, ymd(2026, 12, 1), false},
		{"mixed case", "Current", testNow, ymd(2026, 11, 1), false},
		{"empty", "", testNow, time.Time{}, true},
		{"invalid month", "2026-13", testNow, time.Time{}, true},
		{"full date", "2026-10-15", testNow, time.Time{}, true},
		{"words", "november", testNow, time.Time{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMonth(tt.input, tt.now)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseMonth(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, ErrInvalidDate) {
				t.Errorf("error %v does not wrap ErrInvalidDate", err)
			}
			if !got.Equal(tt.want) {
				t.Errorf("ParseMonth(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
