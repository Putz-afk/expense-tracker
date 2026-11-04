package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/Putz-afk/expense-tracker/internal/expense"
)

// seedPending saves two pending expenses (IDs 1 and 2) and returns nothing:
// #1 is a 22,000 QRIS meal suggested as food, #2 a 400,250 DANA top up.
func seedPending(t *testing.T, env *testEnv) {
	t.Helper()
	s := env.store()
	for _, p := range []expense.Pending{
		{
			MessageID: "<a@x>", Kind: "QRIS Bayar", Payee: "WARUNG MAKAN ELOK",
			Amount: 22000, Category: "food",
			Date: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC),
		},
		{
			MessageID: "<b@x>", Kind: "DANA", Payee: "DANA",
			Amount: 400250, Fee: 250, Category: "other",
			Date: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC),
		},
	} {
		if _, _, err := s.AddPending(p); err != nil {
			t.Fatalf("seed pending: %v", err)
		}
	}
}

func TestInboxListEmpty(t *testing.T) {
	env := newTestEnv(t)
	out, err := env.run("inbox")
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if out != "No pending expenses.\n" {
		t.Errorf("output = %q", out)
	}
}

func TestInboxList(t *testing.T) {
	for _, args := range [][]string{{"inbox"}, {"inbox", "list"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			env := newTestEnv(t)
			seedPending(t, env)

			out, err := env.run(args...)
			if err != nil {
				t.Fatalf("Execute() error: %v", err)
			}
			for _, want := range []string{
				"ID", "SUGGESTED", "WARUNG MAKAN ELOK", "QRIS Bayar", "food",
				"22,000", "400,250", "2026-11-02",
				"2 waiting, total Rp422,250",
				"expense inbox accept <id>",
			} {
				assertContains(t, out, want)
			}
		})
	}
}

func TestInboxAccept(t *testing.T) {
	env := newTestEnv(t)
	seedPending(t, env)

	out, err := env.run("inbox", "accept", "1")
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if want := "Accepted #1 as expense #1: Rp22,000 food (2026-11-02)\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}

	list, err := env.run("list")
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, list, "WARUNG MAKAN ELOK") // the payee becomes the note
	assertContains(t, list, "1 expense, total Rp22,000")

	// It left the queue; the other one is still there.
	queue, err := env.run("inbox")
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, queue, "1 waiting")
	if strings.Contains(queue, "WARUNG") {
		t.Errorf("accepted item still in the queue:\n%s", queue)
	}
}

func TestInboxAcceptSeveralWithCategory(t *testing.T) {
	env := newTestEnv(t)
	seedPending(t, env)

	out, err := env.run("inbox", "accept", "1", "2", "--category", "Shopping")
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	assertContains(t, out, "Accepted #1 as expense #1: Rp22,000 shopping")
	assertContains(t, out, "Accepted #2 as expense #2: Rp400,250 shopping")

	queue, err := env.run("inbox")
	if err != nil {
		t.Fatal(err)
	}
	if queue != "No pending expenses.\n" {
		t.Errorf("queue = %q, want it empty", queue)
	}
}

func TestInboxAcceptRejectErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"accept without id", []string{"inbox", "accept"}, "requires at least 1 arg"},
		{"accept not a number", []string{"inbox", "accept", "abc"}, "invalid pending expense ID"},
		{"accept zero", []string{"inbox", "accept", "0"}, "invalid pending expense ID"},
		{"accept one bad id among good ones", []string{"inbox", "accept", "1", "abc"}, "invalid pending expense ID"},
		{"accept unknown id", []string{"inbox", "accept", "99"}, "pending expense not found: #99"},
		{"accept unknown category", []string{"inbox", "accept", "1", "--category", "nonsense"}, "run `expense categories`"},
		{"reject without id", []string{"inbox", "reject"}, "requires at least 1 arg"},
		{"reject not a number", []string{"inbox", "reject", "x"}, "invalid pending expense ID"},
		{"reject unknown id", []string{"inbox", "reject", "99"}, "pending expense not found: #99"},
		{"stray argument to inbox", []string{"inbox", "bogus"}, "unknown command"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestEnv(t)
			seedPending(t, env)

			_, err := env.run(tt.args...)
			if err == nil {
				t.Fatal("Execute() succeeded, want an error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err, tt.wantErr)
			}

			// Nothing was accepted or rejected along the way.
			queue, err := env.run("inbox")
			if err != nil {
				t.Fatal(err)
			}
			assertContains(t, queue, "2 waiting")
		})
	}
}

func TestInboxReject(t *testing.T) {
	env := newTestEnv(t)
	seedPending(t, env)

	out, err := env.run("inbox", "reject", "2")
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if out != "Rejected #2.\n" {
		t.Errorf("output = %q", out)
	}

	list, err := env.run("list")
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, list, "No expenses found.")

	// Rejecting or accepting it again is an error, not a silent success.
	if _, err := env.run("inbox", "reject", "2"); err == nil || !strings.Contains(err.Error(), "already handled") {
		t.Errorf("second reject error = %v, want 'already handled'", err)
	}
	if _, err := env.run("inbox", "accept", "2"); err == nil || !strings.Contains(err.Error(), "already handled") {
		t.Errorf("accept after reject error = %v, want 'already handled'", err)
	}
}

// The whole flow a user goes through: fetch mail, review, accept.
func TestFetchThenAcceptEndToEnd(t *testing.T) {
	env := newTestEnv(t)
	env.source = &fakeSource{batch: inboxBatch(t, "qris.eml", "transfer_bifast.eml")}

	if _, err := env.run("fetch"); err != nil {
		t.Fatalf("fetch error: %v", err)
	}
	queue, err := env.run("inbox")
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, queue, "2 waiting, total Rp79,500") // 22,000 + 57,500

	// IDs follow fetch order, so #1 is the QRIS meal.
	if _, err := env.run("inbox", "accept", "1"); err != nil {
		t.Fatalf("accept error: %v", err)
	}
	summary, err := env.run("summary", "--month", "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, summary, "food")
	assertContains(t, summary, "22,000")
}
