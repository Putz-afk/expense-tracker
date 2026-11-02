package store

import (
	"database/sql"
	"fmt"
)

// migrations is the ordered list of schema changes. Never edit an entry that
// has already shipped; add a new one at the end instead.
//
// The database remembers how many have been applied in its "user_version"
// header field, so each one runs exactly once.
var migrations = []string{
	// 1: initial schema, plus the starting categories.
	`CREATE TABLE categories (
		name TEXT PRIMARY KEY
	);

	INSERT INTO categories (name) VALUES
		('food'), ('transport'), ('bills'), ('shopping'),
		('health'), ('entertainment'), ('other');

	CREATE TABLE expenses (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		amount     INTEGER NOT NULL CHECK (amount > 0),
		category   TEXT    NOT NULL REFERENCES categories(name),
		note       TEXT    NOT NULL DEFAULT '',
		date       TEXT    NOT NULL,
		created_at TEXT    NOT NULL DEFAULT (datetime('now'))
	);

	CREATE INDEX idx_expenses_date     ON expenses(date);
	CREATE INDEX idx_expenses_category ON expenses(category);`,
}

// migrate brings the database up to the latest schema version.
func migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	for i := version; i < len(migrations); i++ {
		// Each migration runs in a transaction: it fully applies or not at all.
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %d: %w", i+1, err)
		}
		// PRAGMA statements can't take ? parameters, so the number is formatted
		// into the SQL. That's safe here because it's our own int, not user input.
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %d: %w", i+1, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", i+1, err)
		}
	}
	return nil
}
