package store

import (
	"path/filepath"
	"strings"
	"testing"
)

// newTestStore opens a fresh in-memory store that is closed when the test ends.
// t.Helper() makes failures point at the calling test, not at this function.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(MemoryPath)
	if err != nil {
		t.Fatalf("Open(MemoryPath) error: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestOpenSeedsCategories(t *testing.T) {
	s := newTestStore(t)

	rows, err := s.db.Query("SELECT name FROM categories ORDER BY name")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		got = append(got, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	want := "bills,entertainment,food,health,other,shopping,transport"
	if joined := strings.Join(got, ","); joined != want {
		t.Errorf("categories = %s, want %s", joined, want)
	}
}

func TestForeignKeysEnabled(t *testing.T) {
	s := newTestStore(t)

	var on int
	if err := s.db.QueryRow("PRAGMA foreign_keys").Scan(&on); err != nil {
		t.Fatal(err)
	}
	if on != 1 {
		t.Fatalf("PRAGMA foreign_keys = %d, want 1", on)
	}

	tests := []struct {
		name     string
		amount   int64
		category string
		wantErr  bool
	}{
		{"known category", 1000, "food", false},
		{"unknown category", 1000, "nonsense", true},
		{"zero amount", 0, "food", true},
		{"negative amount", -5, "food", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.db.Exec(
				"INSERT INTO expenses (amount, category, date) VALUES (?, ?, '2026-11-02')",
				tt.amount, tt.category)
			if (err != nil) != tt.wantErr {
				t.Errorf("insert error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	s := newTestStore(t)

	// Running migrate again on an up-to-date database must change nothing.
	if err := migrate(s.db); err != nil {
		t.Fatalf("second migrate error: %v", err)
	}

	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != len(migrations) {
		t.Errorf("user_version = %d, want %d", version, len(migrations))
	}

	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM categories").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 7 {
		t.Errorf("category count = %d, want 7 (seed ran twice?)", count)
	}
}

func TestReopenKeepsData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "test.db") // "sub" must be created by Open

	first, err := Open(path)
	if err != nil {
		t.Fatalf("first Open error: %v", err)
	}
	if _, err := first.db.Exec("DELETE FROM categories WHERE name = 'other'"); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := Open(path)
	if err != nil {
		t.Fatalf("second Open error: %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })

	// If the seed re-ran on reopen, "other" would be back.
	var count int
	if err := second.db.QueryRow("SELECT COUNT(*) FROM categories").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 6 {
		t.Errorf("category count after reopen = %d, want 6", count)
	}
}

func TestDefaultPath(t *testing.T) {
	path, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath() error: %v", err)
	}
	if !strings.HasSuffix(path, filepath.Join("expense-tracker", "expenses.db")) {
		t.Errorf("DefaultPath() = %q, want it to end in expense-tracker\\expenses.db", path)
	}
}
