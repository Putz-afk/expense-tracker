package store

import (
	"errors"
	"testing"
	"time"

	"github.com/Putz-afk/expense-tracker/internal/expense"
)

func samplePending(messageID string) expense.Pending {
	return expense.Pending{
		MessageID: messageID,
		Ref:       "243791195082",
		Kind:      "QRIS Bayar",
		Payee:     "WARUNG MAKAN ELOK",
		Subject:   "Pembelian QRIS Berhasil",
		Amount:    22000,
		Category:  "food",
		Date:      date(2026, 11, 2),
	}
}

func mustAddPending(t *testing.T, s *Store, p expense.Pending) int64 {
	t.Helper()
	id, added, err := s.AddPending(p)
	if err != nil {
		t.Fatalf("AddPending() error: %v", err)
	}
	if !added {
		t.Fatalf("AddPending(%q) added = false, want true", p.MessageID)
	}
	return id
}

func TestAddPendingAndList(t *testing.T) {
	s := newTestStore(t)

	// Saved out of date order on purpose: ListPending sorts oldest first.
	later := samplePending("<b@x>")
	later.Date = date(2026, 11, 3)
	later.Category = "  Food " // normalized on the way in
	earlier := samplePending("<a@x>")
	earlier.Fee = 250
	mustAddPending(t, s, later)
	mustAddPending(t, s, earlier)

	got, err := s.ListPending()
	if err != nil {
		t.Fatalf("ListPending() error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListPending() returned %d items, want 2", len(got))
	}
	if got[0].MessageID != "<a@x>" || got[1].MessageID != "<b@x>" {
		t.Errorf("order = %q, %q; want oldest date first", got[0].MessageID, got[1].MessageID)
	}

	first := got[0]
	if first.ID != 2 || first.Amount != 22000 || first.Fee != 250 || first.Category != "food" ||
		first.Payee != "WARUNG MAKAN ELOK" || first.Status != expense.StatusPending ||
		!first.Date.Equal(date(2026, 11, 2)) {
		t.Errorf("first pending = %+v", first)
	}
	if got[1].Category != "food" {
		t.Errorf("category = %q, want food (normalized)", got[1].Category)
	}
}

func TestAddPendingSameEmailTwice(t *testing.T) {
	s := newTestStore(t)

	mustAddPending(t, s, samplePending("<a@x>"))
	id, added, err := s.AddPending(samplePending("<a@x>"))
	if err != nil {
		t.Fatalf("second AddPending() error: %v", err)
	}
	if added || id != 0 {
		t.Errorf("second AddPending() = (%d, %v), want (0, false)", id, added)
	}

	got, err := s.ListPending()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("ListPending() returned %d items, want 1", len(got))
	}
}

func TestAddPendingRejectsBadInput(t *testing.T) {
	s := newTestStore(t)

	tests := []struct {
		name    string
		change  func(*expense.Pending)
		wantErr error
	}{
		{"no message id", func(p *expense.Pending) { p.MessageID = "" }, expense.ErrInvalidExpense},
		{"zero amount", func(p *expense.Pending) { p.Amount = 0 }, expense.ErrInvalidExpense},
		{"fee above amount", func(p *expense.Pending) { p.Fee = 30000 }, expense.ErrInvalidExpense},
		{"no date", func(p *expense.Pending) { p.Date = time.Time{} }, expense.ErrInvalidExpense},
		{"unknown category", func(p *expense.Pending) { p.Category = "nonsense" }, ErrUnknownCategory},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := samplePending("<a@x>")
			tt.change(&p)
			_, added, err := s.AddPending(p)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
			if added {
				t.Error("added = true, want false")
			}
		})
	}
}

func TestAcceptPending(t *testing.T) {
	s := newTestStore(t)
	id := mustAddPending(t, s, samplePending("<a@x>"))

	added, err := s.AcceptPending(id, "")
	if err != nil {
		t.Fatalf("AcceptPending() error: %v", err)
	}
	if added.Amount != 22000 || added.Category != "food" || added.Note != "WARUNG MAKAN ELOK" ||
		!added.Date.Equal(date(2026, 11, 2)) {
		t.Errorf("accepted expense = %+v", added)
	}
	if _, err := s.Get(added.ID); err != nil {
		t.Errorf("accepted expense not stored: %v", err)
	}

	pending, err := s.ListPending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Errorf("ListPending() returned %d items after accept, want 0", len(pending))
	}

	// Accepting twice must not create a second expense.
	if _, err := s.AcceptPending(id, ""); !errors.Is(err, ErrPendingResolved) {
		t.Errorf("second AcceptPending() error = %v, want ErrPendingResolved", err)
	}
	if n := countExpenses(t, s); n != 1 {
		t.Errorf("expense count = %d, want 1", n)
	}
}

func TestAcceptPendingWithOtherCategory(t *testing.T) {
	s := newTestStore(t)
	id := mustAddPending(t, s, samplePending("<a@x>"))

	added, err := s.AcceptPending(id, " Shopping")
	if err != nil {
		t.Fatalf("AcceptPending() error: %v", err)
	}
	if added.Category != "shopping" {
		t.Errorf("category = %q, want shopping", added.Category)
	}
}

func TestAcceptPendingFailureChangesNothing(t *testing.T) {
	s := newTestStore(t)
	id := mustAddPending(t, s, samplePending("<a@x>"))

	if _, err := s.AcceptPending(id, "nonsense"); !errors.Is(err, ErrUnknownCategory) {
		t.Fatalf("AcceptPending() error = %v, want ErrUnknownCategory", err)
	}
	if n := countExpenses(t, s); n != 0 {
		t.Errorf("expense count = %d, want 0", n)
	}
	// Still pending, so the user can try again with a valid category.
	if _, err := s.AcceptPending(id, "food"); err != nil {
		t.Errorf("retry AcceptPending() error: %v", err)
	}
}

func TestAcceptPendingNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.AcceptPending(99, ""); !errors.Is(err, ErrPendingNotFound) {
		t.Errorf("error = %v, want ErrPendingNotFound", err)
	}
}

func TestRejectPending(t *testing.T) {
	s := newTestStore(t)
	id := mustAddPending(t, s, samplePending("<a@x>"))

	if err := s.RejectPending(id); err != nil {
		t.Fatalf("RejectPending() error: %v", err)
	}
	if n := countExpenses(t, s); n != 0 {
		t.Errorf("expense count = %d, want 0", n)
	}

	// The same email arriving again must not come back as a new pending item.
	if _, added, err := s.AddPending(samplePending("<a@x>")); err != nil || added {
		t.Errorf("AddPending() after reject = (added %v, err %v), want (false, nil)", added, err)
	}
	if pending, err := s.ListPending(); err != nil || len(pending) != 0 {
		t.Errorf("ListPending() = %v, %v; want empty", pending, err)
	}

	tests := []struct {
		name    string
		do      func() error
		wantErr error
	}{
		{"reject again", func() error { return s.RejectPending(id) }, ErrPendingResolved},
		{"accept rejected", func() error { _, err := s.AcceptPending(id, ""); return err }, ErrPendingResolved},
		{"unknown id", func() error { return s.RejectPending(99) }, ErrPendingNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.do(); !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestSyncState(t *testing.T) {
	s := newTestStore(t)

	got, err := s.LoadSyncState()
	if err != nil {
		t.Fatalf("LoadSyncState() error: %v", err)
	}
	if got != (SyncState{}) {
		t.Errorf("initial state = %+v, want zero", got)
	}

	want := SyncState{UIDValidity: 1700000000, LastUID: 4242}
	if err := s.SaveSyncState(want); err != nil {
		t.Fatalf("SaveSyncState() error: %v", err)
	}
	got, err = s.LoadSyncState()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("LoadSyncState() = %+v, want %+v", got, want)
	}
}
