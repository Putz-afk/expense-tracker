// Package store persists expenses in SQLite. It is the only package that
// contains SQL.
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	// The blank import (_) runs the package's init(), which registers the
	// "sqlite" driver with database/sql. We never call it directly.
	_ "modernc.org/sqlite"
)

// MemoryPath opens a throwaway in-memory database. Used by tests.
const MemoryPath = ":memory:"

// Store wraps the database connection.
type Store struct {
	db *sql.DB
}

// DefaultPath returns where the database lives for a normal run:
// %AppData%\expense-tracker\expenses.db on Windows.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find config directory: %w", err)
	}
	return filepath.Join(dir, "expense-tracker", "expenses.db"), nil
}

// Open opens (creating if needed) the database at path and migrates it to the
// latest schema.
func Open(path string) (*Store, error) {
	if path != MemoryPath {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}

	// SQLite ignores foreign keys unless enabled per connection. The driver
	// runs this _pragma every time it opens a connection.
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// database/sql keeps a pool of connections, and every ":memory:" connection
	// would be its own separate empty database. One connection avoids that, and
	// is plenty for a single-user CLI.
	db.SetMaxOpenConns(1)

	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	return &Store{db: db}, nil
}

// Close releases the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}
