package cli

import (
	"strings"
	"testing"
)

// seedExpenses adds a few expenses. With the test clock at 2026-11-02:
//
//	#1 2026-10-20 food      30,000  "dinner"
//	#2 2026-11-01 transport 150,000 "grab to airport"
//	#3 2026-11-02 food      25,000  "lunch"
func seedExpenses(t *testing.T, env *testEnv) {
	t.Helper()
	adds := [][]string{
		{"add", "--amount", "30000", "--category", "food", "--note", "dinner", "--date", "2026-10-20"},
		{"add", "--amount", "150000", "--category", "transport", "--note", "grab to airport", "--date", "yesterday"},
		{"add", "--amount", "25000", "--category", "food", "--note", "lunch"},
	}
	for _, args := range adds {
		if _, err := env.run(args...); err != nil {
			t.Fatalf("seed %v: %v", args, err)
		}
	}
}

func TestListCmdTable(t *testing.T) {
	env := newTestEnv(t)
	seedExpenses(t, env)

	out, err := env.run("list")
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	want := "ID  DATE        CATEGORY    AMOUNT  NOTE\n" +
		"3   2026-11-02  food        25,000  lunch\n" +
		"2   2026-11-01  transport  150,000  grab to airport\n" +
		"1   2026-10-20  food        30,000  dinner\n" +
		"\n" +
		"3 expenses, total Rp205,000\n"
	if out != want {
		t.Errorf("output:\n%s\nwant:\n%s", out, want)
	}
}

func TestListCmdFilters(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantIDs   []string // row prefixes that must appear, in order
		wantTotal string
	}{
		{"explicit month", []string{"list", "--month", "2026-10"}, []string{"1 "}, "1 expense, total Rp30,000"},
		{"current month", []string{"list", "--month", "current"}, []string{"3 ", "2 "}, "2 expenses, total Rp175,000"},
		{"last month", []string{"list", "--month", "last"}, []string{"1 "}, "1 expense, total Rp30,000"},
		{"category", []string{"list", "--category", "food"}, []string{"3 ", "1 "}, "2 expenses, total Rp55,000"},
		{"category normalized", []string{"list", "--category", "TRANSPORT"}, []string{"2 "}, "1 expense, total Rp150,000"},
		{"month and category", []string{"list", "--month", "current", "--category", "food"}, []string{"3 "}, "1 expense, total Rp25,000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestEnv(t)
			seedExpenses(t, env)

			out, err := env.run(tt.args...)
			if err != nil {
				t.Fatalf("Execute() error: %v", err)
			}

			// Data rows are the lines between the header and the blank line.
			lines := strings.Split(out, "\n")
			var rows []string
			for _, line := range lines[1:] {
				if line == "" {
					break
				}
				rows = append(rows, line)
			}
			if len(rows) != len(tt.wantIDs) {
				t.Fatalf("got %d rows, want %d:\n%s", len(rows), len(tt.wantIDs), out)
			}
			for i, prefix := range tt.wantIDs {
				if !strings.HasPrefix(rows[i], prefix) {
					t.Errorf("row %d = %q, want prefix %q", i, rows[i], prefix)
				}
			}
			assertContains(t, out, tt.wantTotal)
		})
	}
}

func TestListCmdEmpty(t *testing.T) {
	env := newTestEnv(t)

	tests := [][]string{
		{"list"},
		{"list", "--month", "2020-01"},
		{"list", "--category", "health"},
	}
	for _, args := range tests {
		out, err := env.run(args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if out != "No expenses found.\n" {
			t.Errorf("%v output = %q", args, out)
		}
	}
}

func TestListCmdErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"bad month", []string{"list", "--month", "2026-13"}, "invalid date"},
		{"word month", []string{"list", "--month", "november"}, "use YYYY-MM"},
		{"unknown category", []string{"list", "--category", "nonsense"}, "expense categories"},
		{"stray argument", []string{"list", "oops"}, "unknown command"},
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
