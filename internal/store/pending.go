package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Putz-afk/expense-tracker/internal/expense"
)

var (
	// ErrPendingNotFound is returned (wrapped) when no pending expense has the requested ID.
	ErrPendingNotFound = errors.New("pending expense not found")
	// ErrPendingResolved is returned (wrapped) when a pending expense was already accepted or rejected.
	ErrPendingResolved = errors.New("pending expense already handled")
)

const pendingColumns = "id, message_id, ref, kind, payee, subject, amount, fee, category, date, status"

// checkCategory returns an error wrapping ErrUnknownCategory unless name
// (already normalized) is in the categories table.
func checkCategory(q execer, name string) error {
	var exists int
	err := q.QueryRow("SELECT 1 FROM categories WHERE name = ?", name).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: %q", ErrUnknownCategory, name)
	}
	if err != nil {
		return fmt.Errorf("check category: %w", err)
	}
	return nil
}

// AddPending saves a new pending expense for review. added is false (and
// err nil) when an email with the same MessageID was saved before, whatever
// happened to it since; that is what makes fetching the same mail twice safe.
func (s *Store) AddPending(p expense.Pending) (id int64, added bool, err error) {
	p.Category = expense.NormalizeCategory(p.Category)
	p.Payee = strings.TrimSpace(p.Payee)

	if err := p.Validate(); err != nil {
		return 0, false, err
	}
	if err := checkCategory(s.db, p.Category); err != nil {
		return 0, false, err
	}

	res, err := s.db.Exec(`
		INSERT INTO pending_expenses
			(message_id, ref, kind, payee, subject, amount, fee, category, date)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(message_id) DO NOTHING`,
		p.MessageID, p.Ref, p.Kind, p.Payee, p.Subject, p.Amount, p.Fee,
		p.Category, p.Date.Format(expense.DateLayout))
	if err != nil {
		return 0, false, fmt.Errorf("insert pending expense: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, false, fmt.Errorf("insert pending expense: %w", err)
	}
	if n == 0 {
		return 0, false, nil // duplicate email
	}
	id, err = res.LastInsertId()
	if err != nil {
		return 0, false, fmt.Errorf("read new pending id: %w", err)
	}
	return id, true, nil
}

// ListPending returns the expenses still waiting for review, oldest first.
func (s *Store) ListPending() ([]expense.Pending, error) {
	rows, err := s.db.Query(
		"SELECT "+pendingColumns+" FROM pending_expenses WHERE status = ? ORDER BY date ASC, id ASC",
		string(expense.StatusPending))
	if err != nil {
		return nil, fmt.Errorf("list pending expenses: %w", err)
	}
	defer rows.Close()

	var list []expense.Pending
	for rows.Next() {
		p, err := scanPending(rows)
		if err != nil {
			return nil, fmt.Errorf("read pending expense: %w", err)
		}
		list = append(list, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list pending expenses: %w", err)
	}
	return list, nil
}

// AcceptPending turns a pending expense into a real one and returns it. An
// empty category means "use the suggested one". Saving the expense and marking
// the pending row as accepted happen in one transaction: both or neither.
func (s *Store) AcceptPending(id int64, category string) (expense.Expense, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return expense.Expense{}, fmt.Errorf("begin accept: %w", err)
	}
	// Rollback after a successful Commit just returns an error that we don't
	// care about, so this one deferred call cleans up every early return below.
	defer func() { _ = tx.Rollback() }()

	p, err := getPending(tx, id)
	if err != nil {
		return expense.Expense{}, err
	}
	if p.Status != expense.StatusPending {
		return expense.Expense{}, fmt.Errorf("%w: #%d is %s", ErrPendingResolved, id, p.Status)
	}

	if category == "" {
		category = p.Category
	}
	note := p.Payee
	if note == "" {
		note = p.Kind
	}
	added, err := addExpense(tx, expense.Expense{
		Amount: p.Amount, Category: category, Note: note, Date: p.Date,
	})
	if err != nil {
		return expense.Expense{}, err
	}

	if _, err := tx.Exec("UPDATE pending_expenses SET status = ? WHERE id = ?",
		string(expense.StatusAccepted), id); err != nil {
		return expense.Expense{}, fmt.Errorf("mark pending #%d accepted: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return expense.Expense{}, fmt.Errorf("commit accept of pending #%d: %w", id, err)
	}
	return added, nil
}

// RejectPending marks a pending expense as rejected. The row is kept, so
// fetching the same email again does not bring it back.
func (s *Store) RejectPending(id int64) error {
	res, err := s.db.Exec(
		"UPDATE pending_expenses SET status = ? WHERE id = ? AND status = ?",
		string(expense.StatusRejected), id, string(expense.StatusPending))
	if err != nil {
		return fmt.Errorf("reject pending #%d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("reject pending #%d: %w", id, err)
	}
	if n > 0 {
		return nil
	}

	// Nothing changed: say whether the ID is unknown or already handled.
	p, err := getPending(s.db, id)
	if err != nil {
		return err
	}
	return fmt.Errorf("%w: #%d is %s", ErrPendingResolved, id, p.Status)
}

// SyncState remembers how far the mailbox has been read. IMAP numbers every
// message with a UID, which only stays meaningful while UIDValidity is the
// same; if the server changes it, the saved LastUID must be discarded.
type SyncState struct {
	UIDValidity uint32 // 0 = never synced
	LastUID     uint32 // highest UID already processed
}

// LoadSyncState returns the saved mailbox position.
func (s *Store) LoadSyncState() (SyncState, error) {
	var st SyncState
	err := s.db.QueryRow("SELECT uid_validity, last_uid FROM mail_state WHERE id = 1").
		Scan(&st.UIDValidity, &st.LastUID)
	if err != nil {
		return SyncState{}, fmt.Errorf("load sync state: %w", err)
	}
	return st, nil
}

// SaveSyncState stores the mailbox position.
func (s *Store) SaveSyncState(st SyncState) error {
	_, err := s.db.Exec("UPDATE mail_state SET uid_validity = ?, last_uid = ? WHERE id = 1",
		st.UIDValidity, st.LastUID)
	if err != nil {
		return fmt.Errorf("save sync state: %w", err)
	}
	return nil
}

// getPending reads one pending expense, or returns an error wrapping
// ErrPendingNotFound.
func getPending(q execer, id int64) (expense.Pending, error) {
	row := q.QueryRow("SELECT "+pendingColumns+" FROM pending_expenses WHERE id = ?", id)
	p, err := scanPending(row)
	if errors.Is(err, sql.ErrNoRows) {
		return expense.Pending{}, fmt.Errorf("%w: #%d", ErrPendingNotFound, id)
	}
	if err != nil {
		return expense.Pending{}, fmt.Errorf("get pending #%d: %w", id, err)
	}
	return p, nil
}

func scanPending(sc scanner) (expense.Pending, error) {
	var (
		p       expense.Pending
		dateStr string
		status  string
	)
	err := sc.Scan(&p.ID, &p.MessageID, &p.Ref, &p.Kind, &p.Payee, &p.Subject,
		&p.Amount, &p.Fee, &p.Category, &dateStr, &status)
	if err != nil {
		return expense.Pending{}, err
	}
	date, err := time.Parse(expense.DateLayout, dateStr)
	if err != nil {
		return expense.Pending{}, fmt.Errorf("parse stored date %q: %w", dateStr, err)
	}
	p.Date = date
	p.Status = expense.PendingStatus(status)
	return p, nil
}
