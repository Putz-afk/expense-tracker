package store

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Putz-afk/expense-tracker/internal/expense"
)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func countExpenses(t *testing.T, s *Store) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM expenses").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAddAndGet(t *testing.T) {
	s := newTestStore(t)

	added, err := s.Add(expense.Expense{
		Amount: 25000, Category: "food", Note: "lunch", Date: date(2026, 11, 2),
	})
	if err != nil {
		t.Fatalf("Add() error: %v", err)
	}
	if added.ID != 1 {
		t.Errorf("first ID = %d, want 1", added.ID)
	}

	got, err := s.Get(added.ID)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got.ID != added.ID || got.Amount != 25000 || got.Category != "food" || got.Note != "lunch" {
		t.Errorf("Get() = %+v, want amount 25000, food, lunch", got)
	}
	if want := date(2026, 11, 2); !got.Date.Equal(want) {
		t.Errorf("Get() date = %v, want %v", got.Date, want)
	}

	second, err := s.Add(expense.Expense{Amount: 1000, Category: "other", Date: date(2026, 11, 3)})
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != 2 {
		t.Errorf("second ID = %d, want 2", second.ID)
	}
}

func TestAddNormalizesInput(t *testing.T) {
	s := newTestStore(t)

	added, err := s.Add(expense.Expense{
		Amount: 500, Category: "  Food ", Note: "  snack  ", Date: date(2026, 11, 2),
	})
	if err != nil {
		t.Fatalf("Add() error: %v", err)
	}
	got, err := s.Get(added.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Category != "food" || got.Note != "snack" {
		t.Errorf("stored category/note = %q/%q, want food/snack", got.Category, got.Note)
	}
}

func TestAddRejectsBadInput(t *testing.T) {
	s := newTestStore(t)
	d := date(2026, 11, 2)

	tests := []struct {
		name    string
		in      expense.Expense
		wantErr error
	}{
		{"unknown category", expense.Expense{Amount: 100, Category: "nonsense", Date: d}, ErrUnknownCategory},
		{"empty category", expense.Expense{Amount: 100, Category: "   ", Date: d}, expense.ErrInvalidExpense},
		{"zero amount", expense.Expense{Amount: 0, Category: "food", Date: d}, expense.ErrInvalidExpense},
		{"missing date", expense.Expense{Amount: 100, Category: "food"}, expense.ErrInvalidExpense},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.Add(tt.in)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Add() error = %v, want it to wrap %v", err, tt.wantErr)
			}
		})
	}

	if n := countExpenses(t, s); n != 0 {
		t.Errorf("%d expenses saved after rejected adds, want 0", n)
	}
}

func ids(list []expense.Expense) []int64 {
	out := make([]int64, 0, len(list))
	for _, e := range list {
		out = append(out, e.ID)
	}
	return out
}

func TestList(t *testing.T) {
	s := newTestStore(t)

	// IDs are assigned 1..6 in this order. Rows 3 and 6 share a date.
	seed := []expense.Expense{
		{Amount: 100, Category: "food", Date: date(2026, 10, 31)},
		{Amount: 200, Category: "food", Date: date(2026, 11, 1)},
		{Amount: 300, Category: "transport", Date: date(2026, 11, 15)},
		{Amount: 400, Category: "food", Date: date(2026, 11, 30)},
		{Amount: 500, Category: "food", Date: date(2026, 12, 1)},
		{Amount: 600, Category: "bills", Date: date(2026, 11, 15)},
	}
	for _, e := range seed {
		if _, err := s.Add(e); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name   string
		filter expense.Filter
		want   []int64
	}{
		{"no filter, newest first", expense.Filter{}, []int64{5, 4, 6, 3, 2, 1}},
		{"month includes first and last day", expense.Filter{Month: date(2026, 11, 1)}, []int64{4, 6, 3, 2}},
		{"october", expense.Filter{Month: date(2026, 10, 1)}, []int64{1}},
		{"december", expense.Filter{Month: date(2026, 12, 1)}, []int64{5}},
		{"month with no expenses", expense.Filter{Month: date(2027, 1, 1)}, []int64{}},
		{"category only", expense.Filter{Category: "food"}, []int64{5, 4, 2, 1}},
		{"category is normalized", expense.Filter{Category: " Food "}, []int64{5, 4, 2, 1}},
		{"month and category", expense.Filter{Month: date(2026, 11, 1), Category: "food"}, []int64{4, 2}},
		{"valid category with no expenses", expense.Filter{Category: "health"}, []int64{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.List(tt.filter)
			if err != nil {
				t.Fatalf("List() error: %v", err)
			}
			if !reflect.DeepEqual(ids(got), tt.want) {
				t.Errorf("List() ids = %v, want %v", ids(got), tt.want)
			}
		})
	}
}

func TestListFieldsAndUnknownCategory(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Add(expense.Expense{Amount: 25000, Category: "food", Note: "lunch", Date: date(2026, 11, 2)}); err != nil {
		t.Fatal(err)
	}

	list, err := s.List(expense.Filter{})
	if err != nil || len(list) != 1 {
		t.Fatalf("List() = %v, %v; want one expense", list, err)
	}
	got := list[0]
	if got.ID != 1 || got.Amount != 25000 || got.Category != "food" || got.Note != "lunch" || !got.Date.Equal(date(2026, 11, 2)) {
		t.Errorf("List()[0] = %+v", got)
	}

	if _, err := s.List(expense.Filter{Category: "nonsense"}); !errors.Is(err, ErrUnknownCategory) {
		t.Errorf("unknown category error = %v, want ErrUnknownCategory", err)
	}
}

func TestSummary(t *testing.T) {
	s := newTestStore(t)

	seed := []expense.Expense{
		{Amount: 100, Category: "food", Date: date(2026, 10, 31)},
		{Amount: 200, Category: "food", Date: date(2026, 11, 1)},
		{Amount: 300, Category: "transport", Date: date(2026, 11, 15)},
		{Amount: 400, Category: "food", Date: date(2026, 11, 30)},
		{Amount: 500, Category: "food", Date: date(2026, 12, 1)},
		{Amount: 600, Category: "bills", Date: date(2026, 11, 15)},
	}
	for _, e := range seed {
		if _, err := s.Add(e); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name  string
		month time.Time
		want  []expense.CategoryTotal
	}{
		{
			// food and bills tie at 600, so they are ordered by name.
			name:  "november: totals, counts, tie broken by name",
			month: date(2026, 11, 1),
			want: []expense.CategoryTotal{
				{Category: "bills", Total: 600, Count: 1},
				{Category: "food", Total: 600, Count: 2},
				{Category: "transport", Total: 300, Count: 1},
			},
		},
		{
			name:  "october only has its last day",
			month: date(2026, 10, 1),
			want:  []expense.CategoryTotal{{Category: "food", Total: 100, Count: 1}},
		},
		{
			name:  "december only has its first day",
			month: date(2026, 12, 1),
			want:  []expense.CategoryTotal{{Category: "food", Total: 500, Count: 1}},
		},
		{
			name:  "month with no expenses",
			month: date(2027, 1, 1),
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.Summary(tt.month)
			if err != nil {
				t.Fatalf("Summary() error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Summary() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSummaryNeedsMonth(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Summary(time.Time{}); err == nil {
		t.Error("Summary(zero time) succeeded, want an error")
	}
}

func TestDelete(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 3; i++ {
		if _, err := s.Add(expense.Expense{Amount: 100, Category: "food", Date: date(2026, 11, 2)}); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.Delete(2); err != nil {
		t.Fatalf("Delete(2) error: %v", err)
	}
	if _, err := s.Get(2); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(2) after delete error = %v, want ErrNotFound", err)
	}

	// The other expenses are untouched.
	list, err := s.List(expense.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ids(list), []int64{3, 1}; !reflect.DeepEqual(got, want) {
		t.Errorf("remaining ids = %v, want %v", got, want)
	}

	// Deleting again, or deleting something that never existed, is ErrNotFound.
	for _, id := range []int64{2, 0, -1, 99} {
		if err := s.Delete(id); !errors.Is(err, ErrNotFound) {
			t.Errorf("Delete(%d) error = %v, want ErrNotFound", id, err)
		}
	}
}

func TestDeletedIDsAreNotReused(t *testing.T) {
	s := newTestStore(t)
	first, err := s.Add(expense.Expense{Amount: 100, Category: "food", Date: date(2026, 11, 2)})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(first.ID); err != nil {
		t.Fatal(err)
	}
	second, err := s.Add(expense.Expense{Amount: 100, Category: "food", Date: date(2026, 11, 2)})
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == first.ID {
		t.Errorf("new expense reused deleted ID %d", first.ID)
	}
}

func TestGetNotFound(t *testing.T) {
	s := newTestStore(t)

	for _, id := range []int64{1, 0, -3, 999} {
		if _, err := s.Get(id); !errors.Is(err, ErrNotFound) {
			t.Errorf("Get(%d) error = %v, want it to wrap ErrNotFound", id, err)
		}
	}
}
