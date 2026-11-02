package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/Putz-afk/expense-tracker/internal/store"
)

const prompt2 = "Delete #2 (Rp150,000 transport, 2026-11-01)? [y/N]: "

func TestDeleteCmdConfirmed(t *testing.T) {
	for _, answer := range []string{"y\n", "Y\n", "yes\n", "YES\n", "  yes  \n", "y"} {
		t.Run(strings.TrimSpace(answer), func(t *testing.T) {
			env := newTestEnv(t)
			seedExpenses(t, env)

			out, err := env.runWithInput(answer, "delete", "2")
			if err != nil {
				t.Fatalf("Execute() error: %v", err)
			}
			if want := prompt2 + "Deleted expense #2.\n"; out != want {
				t.Errorf("output = %q, want %q", out, want)
			}

			s := env.store()
			if _, err := s.Get(2); !errors.Is(err, store.ErrNotFound) {
				t.Errorf("#2 still exists after confirmed delete (err = %v)", err)
			}
			// The others are untouched.
			for _, id := range []int64{1, 3} {
				if _, err := s.Get(id); err != nil {
					t.Errorf("#%d was lost: %v", id, err)
				}
			}
		})
	}
}

func TestDeleteCmdCancelled(t *testing.T) {
	// Anything other than y/yes cancels, including no answer at all.
	for _, answer := range []string{"n\n", "N\n", "no\n", "\n", "", "maybe\n", "yep\n", "yy\n", "ye\n"} {
		t.Run("answer="+strings.TrimSpace(answer), func(t *testing.T) {
			env := newTestEnv(t)
			seedExpenses(t, env)

			out, err := env.runWithInput(answer, "delete", "2")
			if err != nil {
				t.Fatalf("Execute() error: %v", err)
			}
			if want := prompt2 + "Cancelled.\n"; out != want {
				t.Errorf("output = %q, want %q", out, want)
			}
			if _, err := env.store().Get(2); err != nil {
				t.Errorf("#2 was deleted even though the answer was %q: %v", answer, err)
			}
		})
	}
}

func TestDeleteCmdErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"no id", []string{"delete"}, "accepts 1 arg"},
		{"two ids", []string{"delete", "1", "2"}, "accepts 1 arg"},
		{"not a number", []string{"delete", "abc"}, "invalid expense ID"},
		{"decimal", []string{"delete", "1.5"}, "invalid expense ID"},
		{"zero", []string{"delete", "0"}, "invalid expense ID"},
		{"negative looks like a flag", []string{"delete", "-1"}, "unknown shorthand flag"},
		{"negative after --", []string{"delete", "--", "-1"}, "invalid expense ID"},
		{"missing", []string{"delete", "99"}, "expense not found: #99"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestEnv(t)
			seedExpenses(t, env)

			// "y" is offered as input to prove errors happen before anything is deleted.
			out, err := env.runWithInput("y\n", tt.args...)
			if err == nil {
				t.Fatal("Execute() succeeded, want an error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err, tt.wantErr)
			}
			if strings.Contains(out, "[y/N]") {
				t.Errorf("prompt was shown for a bad request: %q", out)
			}

			list, err := env.run("list")
			if err != nil {
				t.Fatal(err)
			}
			assertContains(t, list, "3 expenses")
		})
	}
}
