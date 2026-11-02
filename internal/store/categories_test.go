package store

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Putz-afk/expense-tracker/internal/expense"
)

func TestListCategories(t *testing.T) {
	s := newTestStore(t)

	got, err := s.ListCategories()
	if err != nil {
		t.Fatalf("ListCategories() error: %v", err)
	}
	want := []string{"bills", "entertainment", "food", "health", "other", "shopping", "transport"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListCategories() = %v, want %v", got, want)
	}
}

func TestAddCategory(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		want     string
		wantErr  error
		wantList int // number of categories afterwards
	}{
		{"simple", "groceries", "groceries", nil, 8},
		{"normalized", "  Eating   OUT ", "eating out", nil, 8},
		{"duplicate", "food", "", ErrCategoryExists, 7},
		{"duplicate different case", "FOOD", "", ErrCategoryExists, 7},
		{"empty", "   ", "", expense.ErrInvalidCategory, 7},
		{"bad characters", "food!", "", expense.ErrInvalidCategory, 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)

			got, err := s.AddCategory(tt.input)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("AddCategory(%q) error = %v, want %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("AddCategory(%q) = %q, want %q", tt.input, got, tt.want)
			}

			list, err := s.ListCategories()
			if err != nil {
				t.Fatal(err)
			}
			if len(list) != tt.wantList {
				t.Errorf("%d categories afterwards, want %d: %v", len(list), tt.wantList, list)
			}
		})
	}
}

func TestNewCategoryUsableForExpenses(t *testing.T) {
	s := newTestStore(t)

	if _, err := s.Add(expense.Expense{Amount: 100, Category: "groceries", Date: date(2026, 11, 2)}); !errors.Is(err, ErrUnknownCategory) {
		t.Fatalf("before adding the category, error = %v, want ErrUnknownCategory", err)
	}
	if _, err := s.AddCategory("groceries"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add(expense.Expense{Amount: 100, Category: "Groceries", Date: date(2026, 11, 2)}); err != nil {
		t.Errorf("after adding the category, Add() error: %v", err)
	}
}
