package cli

import (
	"strings"
	"testing"
	"time"
)

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("output %q does not contain %q", got, want)
	}
}

func TestAddCmdSuccess(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantOut  string
		wantNote string
		wantDate time.Time
	}{
		{
			name:     "defaults to today",
			args:     []string{"add", "--amount", "25000", "--category", "food", "--note", "lunch"},
			wantOut:  "Added expense #1: Rp25,000 food (2026-11-02)",
			wantNote: "lunch",
			wantDate: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "yesterday",
			args:     []string{"add", "--amount", "150000", "--category", "transport", "--date", "yesterday"},
			wantOut:  "Added expense #1: Rp150,000 transport (2026-11-01)",
			wantDate: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "explicit date and uppercase category",
			args:     []string{"add", "--amount", "80000", "--category", "Bills", "--date", "2026-10-28"},
			wantOut:  "Added expense #1: Rp80,000 bills (2026-10-28)",
			wantDate: time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestEnv(t)
			out, err := env.run(tt.args...)
			if err != nil {
				t.Fatalf("Execute() error: %v", err)
			}
			assertContains(t, out, tt.wantOut)

			saved, err := env.store().Get(1)
			if err != nil {
				t.Fatalf("expense was not saved: %v", err)
			}
			if saved.Note != tt.wantNote {
				t.Errorf("saved note = %q, want %q", saved.Note, tt.wantNote)
			}
			if !saved.Date.Equal(tt.wantDate) {
				t.Errorf("saved date = %v, want %v", saved.Date, tt.wantDate)
			}
		})
	}
}

func TestAddCmdErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"missing amount", []string{"add", "--category", "food"}, `"amount" not set`},
		{"missing category", []string{"add", "--amount", "100"}, `"category" not set`},
		{"decimal amount", []string{"add", "--amount", "12.50", "--category", "food"}, "whole number"},
		{"zero amount", []string{"add", "--amount", "0", "--category", "food"}, "greater than zero"},
		{"bad date", []string{"add", "--amount", "100", "--category", "food", "--date", "tomorrow"}, "invalid date"},
		{"unknown category", []string{"add", "--amount", "100", "--category", "nonsense"}, "expense categories"},
		{"stray argument", []string{"add", "oops", "--amount", "100", "--category", "food"}, "unknown command"},
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

func TestAddCmdSavesNothingOnError(t *testing.T) {
	env := newTestEnv(t)
	if _, err := env.run("add", "--amount", "100", "--category", "nonsense"); err == nil {
		t.Fatal("want an error for an unknown category")
	}

	if _, err := env.store().Get(1); err == nil {
		t.Error("an expense was saved even though the command failed")
	}
}
