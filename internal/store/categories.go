package store

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/Putz-afk/expense-tracker/internal/expense"
)

// ErrCategoryExists is returned (wrapped) when adding a category that is already there.
var ErrCategoryExists = errors.New("category already exists")

// ListCategories returns all category names in alphabetical order.
func (s *Store) ListCategories() ([]string, error) {
	rows, err := s.db.Query("SELECT name FROM categories ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("query categories: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("read category: %w", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read categories: %w", err)
	}
	return names, nil
}

// AddCategory normalizes and validates name, saves it, and returns the stored
// (normalized) name. Adding an existing category returns ErrCategoryExists.
func (s *Store) AddCategory(name string) (string, error) {
	name = expense.NormalizeCategory(name)
	if err := expense.ValidateCategoryName(name); err != nil {
		return "", err
	}

	var exists int
	err := s.db.QueryRow("SELECT 1 FROM categories WHERE name = ?", name).Scan(&exists)
	if err == nil {
		return "", fmt.Errorf("%w: %q", ErrCategoryExists, name)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("check category: %w", err)
	}

	if _, err := s.db.Exec("INSERT INTO categories (name) VALUES (?)", name); err != nil {
		return "", fmt.Errorf("insert category: %w", err)
	}
	return name, nil
}
