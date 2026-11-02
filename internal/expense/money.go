// Package expense holds the core domain logic: what an expense is and how
// money is parsed and displayed. It knows nothing about SQL or the CLI.
package expense

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrInvalidAmount is returned (wrapped) when an amount string can't be used.
// A package-level error value like this is called a "sentinel error": callers
// check for it with errors.Is instead of comparing message text.
var ErrInvalidAmount = errors.New("invalid amount")

// ParseAmount converts user input such as "25000" into whole rupiah.
// Only positive whole numbers are accepted.
func ParseAmount(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("%w: amount is empty", ErrInvalidAmount)
	}
	if strings.ContainsAny(s, ".,") {
		return 0, fmt.Errorf("%w: %q must be a whole number without decimals or separators", ErrInvalidAmount, s)
	}

	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q is not a number", ErrInvalidAmount, s)
	}
	if n <= 0 {
		return 0, fmt.Errorf("%w: %q must be greater than zero", ErrInvalidAmount, s)
	}
	return n, nil
}

// FormatAmount renders whole rupiah with thousands separators: 25000 -> "25,000".
func FormatAmount(n int64) string {
	digits := strconv.FormatInt(n, 10)
	sign := ""
	if n < 0 {
		sign = "-"
		digits = digits[1:]
	}

	// Insert a comma before every group of three digits, counting from the right.
	var b strings.Builder
	b.WriteString(sign)
	for i, r := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String()
}
