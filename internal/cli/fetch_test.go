package cli

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Putz-afk/expense-tracker/internal/inbox"
	"github.com/Putz-afk/expense-tracker/internal/store"
)

// fakeSource is a fake mailbox. FetchSince returns batch and err, unless
// fetch is set. WaitForMail blocks until the context is cancelled, unless
// wait is set.
type fakeSource struct {
	batch inbox.Batch
	err   error
	fetch func(call int) (inbox.Batch, error) // call counts from 0
	wait  func(ctx context.Context) error

	fetchCalls int
}

func (f *fakeSource) FetchSince(context.Context, store.SyncState) (inbox.Batch, error) {
	call := f.fetchCalls
	f.fetchCalls++
	if f.fetch != nil {
		return f.fetch(call)
	}
	return f.batch, f.err
}

func (f *fakeSource) WaitForMail(ctx context.Context) error {
	if f.wait != nil {
		return f.wait(ctx)
	}
	<-ctx.Done()
	return ctx.Err()
}

func emailFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("../bri/testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return raw
}

// inboxBatch builds a batch from fixture files, with UIDs 1, 2, 3, ...
func inboxBatch(t *testing.T, files ...string) inbox.Batch {
	t.Helper()
	batch := inbox.Batch{UIDValidity: 1}
	for i, f := range files {
		batch.Messages = append(batch.Messages, inbox.Message{UID: uint32(i + 1), Raw: emailFixture(t, f)})
	}
	return batch
}

const otpEmail = "From: Bank BRI <BankBRI@bri.co.id>\nSubject: OTP\nContent-Type: text/html\n\n<p>Kode OTP 123456</p>"
const brokenEmail = "From: Bank BRI <BankBRI@bri.co.id>\nSubject: x\nContent-Type: text/html\n\n<td>Total Transaksi</td><td>abc</td>"

func TestFetchCmd(t *testing.T) {
	env := newTestEnv(t)
	env.source = &fakeSource{batch: inbox.Batch{
		UIDValidity: 1,
		Messages: []inbox.Message{
			{UID: 1, Raw: emailFixture(t, "qris.eml")},
			{UID: 2, Raw: []byte(otpEmail)},
			{UID: 3, Raw: emailFixture(t, "topup_dana.eml")},
		},
	}}

	out, err := env.run("fetch")
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	want := "2 new, 0 already saved, 1 skipped (not transactions)\n" +
		"2 waiting for review. Run `expense inbox` to see them.\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}

	// Fetching the same mail again changes nothing.
	out, err = env.run("fetch")
	if err != nil {
		t.Fatalf("second Execute() error: %v", err)
	}
	assertContains(t, out, "0 new, 2 already saved, 1 skipped")
	assertContains(t, out, "2 waiting for review")

	// Fetching never creates real expenses by itself.
	list, err := env.run("list")
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, list, "No expenses found.")
}

func TestFetchCmdNothingNew(t *testing.T) {
	env := newTestEnv(t)
	env.source = &fakeSource{batch: inbox.Batch{UIDValidity: 1}}

	out, err := env.run("fetch")
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	want := "0 new, 0 already saved, 0 skipped (not transactions)\nNothing waiting for review.\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

func TestFetchCmdWarnsAboutUnreadableMail(t *testing.T) {
	env := newTestEnv(t)
	env.source = &fakeSource{batch: inbox.Batch{
		UIDValidity: 1,
		Messages: []inbox.Message{
			{UID: 5, Raw: []byte(brokenEmail)},
			{UID: 6, Raw: emailFixture(t, "qris.eml")},
		},
	}}

	out, err := env.run("fetch")
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	assertContains(t, out, "warning: could not read message 5")
	assertContains(t, out, "1 new") // the broken mail didn't stop the good one
}

func TestFetchCmdSince(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want time.Time
	}{
		{"default is the start of this month", []string{"fetch"}, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)},
		{"explicit date", []string{"fetch", "--since", "2026-10-15"}, time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)},
		{"yesterday", []string{"fetch", "--since", "yesterday"}, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestEnv(t)
			env.source = &fakeSource{}
			if _, err := env.run(tt.args...); err != nil {
				t.Fatalf("Execute() error: %v", err)
			}
			if !env.gotSince.Equal(tt.want) {
				t.Errorf("since = %v, want %v", env.gotSince, tt.want)
			}
		})
	}
}

func TestFetchCmdErrors(t *testing.T) {
	t.Run("bad --since", func(t *testing.T) {
		env := newTestEnv(t)
		env.source = &fakeSource{}
		_, err := env.run("fetch", "--since", "last tuesday")
		if err == nil || !strings.Contains(err.Error(), "invalid date") {
			t.Errorf("error = %v, want an invalid date error", err)
		}
		if env.sourceOpen != 0 {
			t.Error("mailbox was opened despite the bad flag")
		}
	})

	t.Run("credentials missing", func(t *testing.T) {
		env := newTestEnv(t)
		env.sourceErr = errors.New("set EXPENSE_IMAP_USER and EXPENSE_IMAP_PASSWORD")
		_, err := env.run("fetch")
		if err == nil || !strings.Contains(err.Error(), "EXPENSE_IMAP_USER") {
			t.Errorf("error = %v, want the credentials hint", err)
		}
	})

	t.Run("mail server fails", func(t *testing.T) {
		env := newTestEnv(t)
		env.source = &fakeSource{err: errors.New("connection refused")}
		_, err := env.run("fetch")
		if err == nil || !strings.Contains(err.Error(), "connection refused") {
			t.Errorf("error = %v, want it to mention the cause", err)
		}
	})

	t.Run("takes no arguments", func(t *testing.T) {
		env := newTestEnv(t)
		env.source = &fakeSource{}
		if _, err := env.run("fetch", "extra"); err == nil {
			t.Error("Execute() succeeded, want an error")
		}
	})
}
