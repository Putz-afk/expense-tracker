package cli

import (
	"strings"
	"testing"
)

func TestCategoriesCmdLists(t *testing.T) {
	env := newTestEnv(t)

	out, err := env.run("categories")
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	want := "bills\nentertainment\nfood\nhealth\nother\nshopping\ntransport\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

func TestCategoriesAddCmd(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantOut string
	}{
		{"one word", []string{"categories", "add", "groceries"}, `Added category "groceries"`},
		{"several words without quotes", []string{"categories", "add", "Eating", "Out"}, `Added category "eating out"`},
		{"quoted", []string{"categories", "add", "  gym   2026 "}, `Added category "gym 2026"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestEnv(t)
			out, err := env.run(tt.args...)
			if err != nil {
				t.Fatalf("Execute() error: %v", err)
			}
			assertContains(t, out, tt.wantOut)

			// The new category shows up in the list...
			list, err := env.run("categories")
			if err != nil {
				t.Fatal(err)
			}
			name := strings.TrimSuffix(strings.TrimPrefix(tt.wantOut, `Added category "`), `"`)
			assertContains(t, list, name+"\n")
		})
	}
}

func TestCategoriesAddThenUseInExpense(t *testing.T) {
	env := newTestEnv(t)

	if _, err := env.run("add", "--amount", "5000", "--category", "groceries"); err == nil {
		t.Fatal("add with an unknown category should fail")
	}
	if _, err := env.run("categories", "add", "groceries"); err != nil {
		t.Fatal(err)
	}
	out, err := env.run("add", "--amount", "5000", "--category", "groceries")
	if err != nil {
		t.Fatalf("add after creating the category failed: %v", err)
	}
	assertContains(t, out, "Rp5,000 groceries")
}

func TestCategoriesCmdErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"duplicate", []string{"categories", "add", "food"}, "already exists"},
		{"duplicate different case", []string{"categories", "add", "FOOD"}, "already exists"},
		{"no name", []string{"categories", "add"}, "requires at least 1 arg"},
		{"bad characters", []string{"categories", "add", "food!"}, "may only contain"},
		{"too long", []string{"categories", "add", strings.Repeat("x", 31)}, "longer than"},
		{"stray argument to list", []string{"categories", "extra"}, "unknown command"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestEnv(t)
			_, err := env.run(tt.args...)
			if err == nil {
				t.Fatal("Execute() succeeded, want an error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err, tt.wantErr)
			}
		})
	}
}
