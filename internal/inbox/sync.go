// Package inbox moves bank emails into the pending queue: it asks a mail
// Source for new messages, parses them and saves what it finds.
package inbox

import (
	"context"
	"errors"
	"fmt"

	"github.com/Putz-afk/expense-tracker/internal/bri"
	"github.com/Putz-afk/expense-tracker/internal/expense"
	"github.com/Putz-afk/expense-tracker/internal/store"
)

// Message is one raw email and the UID the mail server gave it.
type Message struct {
	UID uint32
	Raw []byte
}

// Batch is what a Source found: the messages (oldest first) and the
// mailbox's current UIDValidity.
type Batch struct {
	UIDValidity uint32
	Messages    []Message
}

// Source is anything that can list new bank emails. The real one talks IMAP
// (package mail); tests use a fake.
//
// FetchSince returns the messages with a UID above st.LastUID. If the
// server's UIDValidity differs from st.UIDValidity, the saved UIDs mean
// nothing any more, so the Source must start from the beginning instead.
// Re-reading old mail is harmless: Sync never saves the same email twice.
//
// In Go, the interface is defined by the code that uses it (here), not by
// the code that implements it. That keeps this package free of IMAP.
type Source interface {
	FetchSince(ctx context.Context, st store.SyncState) (Batch, error)
}

// Failure is an email that could not be parsed.
type Failure struct {
	UID uint32
	Err error
}

// Result counts what one Sync did.
type Result struct {
	Added      int // new pending expenses
	Duplicates int // transactions that were already saved
	Skipped    int // emails that are not transactions (OTP, promotions, ...)
	Failed     []Failure
}

// Sync saves the transactions in new bank emails as pending expenses.
//
// An email that can't be parsed is reported in Result.Failed and skipped, so
// one odd email never blocks the rest. A database error stops the run and the
// mailbox position is not saved; the next Sync starts again from the same
// place, and duplicates are ignored.
func Sync(ctx context.Context, src Source, s *store.Store) (Result, error) {
	var res Result

	st, err := s.LoadSyncState()
	if err != nil {
		return res, err
	}
	batch, err := src.FetchSince(ctx, st)
	if err != nil {
		return res, fmt.Errorf("fetch mail: %w", err)
	}

	// A new UIDValidity means the old position is void: count from zero.
	next := store.SyncState{UIDValidity: batch.UIDValidity}
	if st.UIDValidity == batch.UIDValidity {
		next.LastUID = st.LastUID
	}

	for _, m := range batch.Messages {
		if err := ctx.Err(); err != nil {
			return res, err
		}

		tx, err := bri.Parse(m.Raw)
		switch {
		case errors.Is(err, bri.ErrNotATransaction):
			res.Skipped++
		case err != nil:
			res.Failed = append(res.Failed, Failure{UID: m.UID, Err: err})
		default:
			_, added, err := s.AddPending(expense.Pending{
				MessageID: tx.MessageID,
				Ref:       tx.Ref,
				Kind:      tx.Kind,
				Payee:     tx.Payee,
				Subject:   tx.Subject,
				Amount:    tx.Amount,
				Fee:       tx.Fee,
				Category:  bri.SuggestCategory(tx),
				Date:      tx.Date,
			})
			if err != nil {
				return res, fmt.Errorf("save message %d: %w", m.UID, err)
			}
			if added {
				res.Added++
			} else {
				res.Duplicates++
			}
		}

		next.LastUID = max(next.LastUID, m.UID)
	}

	if err := s.SaveSyncState(next); err != nil {
		return res, err
	}
	return res, nil
}
