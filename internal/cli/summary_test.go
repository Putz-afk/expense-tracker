package cli

import (
	"strings"
	"testing"
)

func TestSummaryCmdOutput(t *testing.T) {
	// seedExpenses: #1 food 30,000 (Oct 20); #2 transport 150,000 (Nov 1); #3 food 25,000 (Nov 2).
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "defaults to the current month",
			args: []string{"summary"},
			want: "November 2026\n" +
				"\n" +
				"CATEGORY    AMOUNT  COUNT\n" +
				"transport  150,000      1\n" +
				"food        25,000      1\n" +
				"TOTAL      175,000      2\n",
		},
		{
			name: "last month",
			args: []string{"summary", "--month", "last"},
			want: "October 2026\n" +
				"\n" +
				"CATEGORY  AMOUNT  COUNT\n" +
				"food      30,000      1\n" +
				"TOTAL     30,000      1\n",
		},
		{
			name: "explicit month",
			args: []string{"summary", "--month", "2026-10"},
			want: "October 2026\n" +
				"\n" +
				"CATEGORY  AMOUNT  COUNT\n" +
				"food      30,000      1\n" +
				"TOTAL     30,000      1\n",
		},
		{
			name: "month without expenses",
			args: []string{"summary", "--month", "2020-01"},
			want: "No expenses in January 2020.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestEnv(t)
			seedExpenses(t, env)

			out, err := env.run(tt.args...)
			if err != nil {
				t.Fatalf("Execute() error: %v", err)
			}
			if out != tt.want {
				t.Errorf("output:\n%s\nwant:\n%s", out, tt.want)
			}
		})
	}
}

func TestSummaryCmdEmptyDatabase(t *testing.T) {
	env := newTestEnv(t)
	out, err := env.run("summary")
	if err != nil {
		t.Fatal(err)
	}
	if want := "No expenses in November 2026.\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

func TestSummaryCmdErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"bad month", []string{"summary", "--month", "2026-13"}, "invalid date"},
		{"word month", []string{"summary", "--month", "november"}, "use YYYY-MM"},
		{"empty month", []string{"summary", "--month", ""}, "invalid date"},
		{"stray argument", []string{"summary", "oops"}, "unknown command"},
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
