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
	// ErrUnknownCategory is returned (wrapped) when a category isn't in the categories table.
	ErrUnknownCategory = errors.New("unknown category")
	// ErrNotFound is returned (wrapped) when no expense has the requested ID.
	ErrNotFound = errors.New("expense not found")
)

// execer is the part of *sql.DB and *sql.Tx that addExpense needs. Both types
// have these methods, so Go lets either one be passed in without any
// declaration saying so (interfaces are satisfied implicitly).
type execer interface {
	QueryRow(query string, args ...any) *sql.Row
	Exec(query string, args ...any) (sql.Result, error)
}

// Add validates and saves a new expense, and returns it with its ID filled in.
// The category is normalized (trimmed, lowercased) before it is checked.
func (s *Store) Add(e expense.Expense) (expense.Expense, error) {
	return addExpense(s.db, e)
}

// addExpense is Add's body. It takes an execer so AcceptPending can run it
// inside a transaction (with one connection, calling s.Add there would wait
// forever for the connection the transaction is holding).
func addExpense(q execer, e expense.Expense) (expense.Expense, error) {
	e.Category = expense.NormalizeCategory(e.Category)
	e.Note = strings.TrimSpace(e.Note)

	if err := e.Validate(); err != nil {
		return expense.Expense{}, err
	}

	// The foreign key would also reject a bad category, but its error message
	// is cryptic. Checking first lets us return a clear one.
	if err := checkCategory(q, e.Category); err != nil {
		return expense.Expense{}, err
	}

	res, err := q.Exec(
		"INSERT INTO expenses (amount, category, note, date) VALUES (?, ?, ?, ?)",
		e.Amount, e.Category, e.Note, e.Date.Format(expense.DateLayout),
	)
	if err != nil {
		return expense.Expense{}, fmt.Errorf("insert expense: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return expense.Expense{}, fmt.Errorf("read new expense id: %w", err)
	}
	e.ID = id
	return e, nil
}

// Get returns the expense with the given ID, or an error wrapping ErrNotFound.
func (s *Store) Get(id int64) (expense.Expense, error) {
	row := s.db.QueryRow(
		"SELECT id, amount, category, note, date FROM expenses WHERE id = ?", id)

	e, err := scanExpense(row)
	if errors.Is(err, sql.ErrNoRows) {
		return expense.Expense{}, fmt.Errorf("%w: #%d", ErrNotFound, id)
	}
	if err != nil {
		return expense.Expense{}, fmt.Errorf("get expense #%d: %w", id, err)
	}
	return e, nil
}

// Delete removes the expense with the given ID, or returns an error wrapping
// ErrNotFound if there is none.
func (s *Store) Delete(id int64) error {
	res, err := s.db.Exec("DELETE FROM expenses WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete expense #%d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete expense #%d: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("%w: #%d", ErrNotFound, id)
	}
	return nil
}

// List returns the expenses matching f, newest first (ties broken by newest ID).
// A category filter that isn't a known category returns ErrUnknownCategory
// rather than an empty list, so typos don't look like "no spending".
func (s *Store) List(f expense.Filter) ([]expense.Expense, error) {
	query := "SELECT id, amount, category, note, date FROM expenses"
	var (
		conds []string
		args  []any
	)

	if !f.Month.IsZero() {
		start, end := monthRange(f.Month)
		conds = append(conds, "date >= ? AND date < ?")
		args = append(args, start, end)
	}

	if f.Category != "" {
		category := expense.NormalizeCategory(f.Category)
		if err := checkCategory(s.db, category); err != nil {
			return nil, err
		}
		conds = append(conds, "category = ?")
		args = append(args, category)
	}

	// Only fixed strings are joined into the SQL; every user-supplied value
	// travels in args as a ? parameter.
	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}
	query += " ORDER BY date DESC, id DESC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list expenses: %w", err)
	}
	defer rows.Close()

	var list []expense.Expense
	for rows.Next() {
		e, err := scanExpense(rows)
		if err != nil {
			return nil, fmt.Errorf("read expense: %w", err)
		}
		list = append(list, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list expenses: %w", err)
	}
	return list, nil
}

// monthRange returns the stored-date bounds of the month starting at first:
// every date d in the month satisfies start <= d < end. ISO dates sort like
// text, so a plain range comparison in SQL selects the month.
func monthRange(first time.Time) (start, end string) {
	return first.Format(expense.DateLayout), first.AddDate(0, 1, 0).Format(expense.DateLayout)
}

// Summary returns the total spent per category in the month starting at
// month, largest total first (ties by category name). Categories with no
// expenses that month are left out.
func (s *Store) Summary(month time.Time) ([]expense.CategoryTotal, error) {
	if month.IsZero() {
		return nil, errors.New("summary needs a month")
	}
	start, end := monthRange(month)

	rows, err := s.db.Query(`
		SELECT category, SUM(amount), COUNT(*)
		FROM expenses
		WHERE date >= ? AND date < ?
		GROUP BY category
		ORDER BY SUM(amount) DESC, category ASC`, start, end)
	if err != nil {
		return nil, fmt.Errorf("summarize expenses: %w", err)
	}
	defer rows.Close()

	var totals []expense.CategoryTotal
	for rows.Next() {
		var t expense.CategoryTotal
		if err := rows.Scan(&t.Category, &t.Total, &t.Count); err != nil {
			return nil, fmt.Errorf("read summary row: %w", err)
		}
		totals = append(totals, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("summarize expenses: %w", err)
	}
	return totals, nil
}

// scanner is satisfied by both *sql.Row and *sql.Rows, so one function can
// read an expense from a single-row query (Get) or a multi-row one (List).
type scanner interface {
	Scan(dest ...any) error
}

func scanExpense(sc scanner) (expense.Expense, error) {
	var (
		e       expense.Expense
		dateStr string
	)
	if err := sc.Scan(&e.ID, &e.Amount, &e.Category, &e.Note, &dateStr); err != nil {
		return expense.Expense{}, err
	}
	date, err := time.Parse(expense.DateLayout, dateStr)
	if err != nil {
		return expense.Expense{}, fmt.Errorf("parse stored date %q: %w", dateStr, err)
	}
	e.Date = date
	return e, nil
}
