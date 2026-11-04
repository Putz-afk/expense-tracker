package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Putz-afk/expense-tracker/internal/inbox"
)

func TestListenCmdQueuesMailUntilCancelled(t *testing.T) {
	env := newTestEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	waits := 0
	env.source = &fakeSource{
		fetch: func(call int) (inbox.Batch, error) {
			switch call {
			case 0:
				return inboxBatch(t, "qris.eml"), nil
			case 1:
				// Nothing new this time: must not print anything.
				return inbox.Batch{UIDValidity: 1}, nil
			default:
				b := inboxBatch(t, "transfer_bifast.eml")
				b.Messages[0].UID = 2
				return b, nil
			}
		},
		wait: func(ctx context.Context) error {
			waits++
			if waits <= 2 {
				return nil // "new mail arrived"
			}
			cancel() // the user presses Ctrl+C
			<-ctx.Done()
			return ctx.Err()
		},
	}

	out, err := env.runContext(ctx, "", "listen")
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	assertContains(t, out, "Listening for Bank BRI emails. Press Ctrl+C to stop.")
	assertContains(t, out, "1 new, 0 already saved, 0 skipped")
	assertContains(t, out, "Run `expense inbox` to review.")
	if !strings.HasSuffix(out, "Stopped.\n") {
		t.Errorf("output should end with Stopped.:\n%s", out)
	}
	// Three syncs ran, but only the two that found something are printed.
	if n := strings.Count(out, "] 1 new"); n != 2 {
		t.Errorf("printed %d result lines, want 2 (the quiet sync prints nothing):\n%s", n, out)
	}

	queue, err := env.run("inbox")
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, queue, "2 waiting, total Rp79,500")
}

func TestListenCmdKeepsRunningAfterErrors(t *testing.T) {
	env := newTestEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The first sync fails; Run then waits out its real 5 second backoff, so
	// cancel right after the error was reported instead of waiting for it.
	src := &fakeSource{
		fetch: func(call int) (inbox.Batch, error) {
			go cancel()
			return inbox.Batch{}, errors.New("connection refused")
		},
	}
	env.source = src

	out, err := env.runContext(ctx, "", "listen")
	if err != nil {
		t.Fatalf("Execute() error: %v (an error while listening must not end the command)", err)
	}
	// Whether the error line or the cancellation wins the race is up to the
	// scheduler; either way the command must stop cleanly.
	assertContains(t, out, "Stopped.")
}

func TestListenCmdErrors(t *testing.T) {
	t.Run("bad --since", func(t *testing.T) {
		env := newTestEnv(t)
		env.source = &fakeSource{}
		_, err := env.run("listen", "--since", "nope")
		if err == nil || !strings.Contains(err.Error(), "invalid date") {
			t.Errorf("error = %v, want an invalid date error", err)
		}
	})

	t.Run("credentials missing", func(t *testing.T) {
		env := newTestEnv(t)
		env.sourceErr = errors.New("set EXPENSE_IMAP_USER and EXPENSE_IMAP_PASSWORD")
		_, err := env.run("listen")
		if err == nil || !strings.Contains(err.Error(), "EXPENSE_IMAP_USER") {
			t.Errorf("error = %v, want the credentials hint", err)
		}
	})
}
