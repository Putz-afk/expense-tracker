// Package bri turns Bank BRI (BRImo) notification emails into Transactions.
// It does no network or database work: bytes in, struct out.
package bri

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"io"
	"mime/quotedprintable"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Putz-afk/expense-tracker/internal/expense"
)

// ErrNotATransaction is returned when an email has no transaction total in it
// (an OTP, a promotion, ...). Callers should skip such mail quietly.
var ErrNotATransaction = errors.New("not a transaction email")

// Transaction is the part of a BRI notification we care about.
type Transaction struct {
	MessageID string    // the email's Message-ID without <>, unique per email
	Subject   string    // e.g. "Pembelian QRIS Berhasil"
	Ref       string    // BRI's reference number ("Nomor Referensi")
	Kind      string    // "Jenis Transaksi", e.g. "QRIS Bayar"
	Payee     string    // merchant or recipient name; falls back to Kind
	Amount    int64     // total that left the account, fee included, whole rupiah
	Fee       int64     // admin fee already included in Amount
	Date      time.Time // calendar date as midnight UTC, like expense.Expense.Date
}

// The three BRI layouts differ a lot (labels sit in <th>, in <td>, or nowhere),
// but after stripping the HTML each label is a line followed by its value on
// the next line. So one label-based parser handles all of them.
//
// regexp.MustCompile panics on a bad pattern. That is fine at package level:
// it runs once at startup, so a typo shows up on the first test run.
var (
	styleRe = regexp.MustCompile(`(?is)<(style|script).*?</(style|script)>`)
	tagRe   = regexp.MustCompile(`<[^>]*>`)
	dateRe  = regexp.MustCompile(`(\d{1,2})\s+([A-Za-z]+)\s+(\d{4})\s*,\s*\d{1,2}:\d{2}(?::\d{2})?\s*W[A-Z]{1,3}`)
)

// months maps the first three letters of a month name, in Indonesian or
// English, to the month. "Oktober" -> "okt", "Agustus" -> "agu".
var months = map[string]time.Month{
	"jan": time.January, "feb": time.February, "mar": time.March,
	"apr": time.April, "mei": time.May, "may": time.May,
	"jun": time.June, "jul": time.July, "agu": time.August,
	"aug": time.August, "sep": time.September, "okt": time.October,
	"oct": time.October, "nov": time.November, "des": time.December,
	"dec": time.December,
}

// Parse reads one raw email (headers and body, as downloaded from IMAP).
// It returns ErrNotATransaction for mail that has no transaction total.
func Parse(raw []byte) (Transaction, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return Transaction{}, fmt.Errorf("read email: %w", err)
	}

	// msg.Body is an io.Reader. Wrapping one reader in another is the usual Go
	// way to chain decoders: bytes flow through quotedprintable on their way to
	// io.ReadAll. BRI sends charset Cp1252, but every character we read is plain
	// ASCII, so we don't convert it.
	var body io.Reader = msg.Body
	if strings.EqualFold(msg.Header.Get("Content-Transfer-Encoding"), "quoted-printable") {
		body = quotedprintable.NewReader(body)
	}
	b, err := io.ReadAll(body)
	if err != nil {
		return Transaction{}, fmt.Errorf("decode email body: %w", err)
	}
	lines := textLines(string(b))

	totalStr, ok := valueAfter(lines, "Total Transaksi", "Total")
	if !ok {
		return Transaction{}, ErrNotATransaction
	}
	amount, err := parseRupiah(totalStr)
	if err != nil {
		return Transaction{}, fmt.Errorf("read total: %w", err)
	}
	if amount <= 0 {
		return Transaction{}, fmt.Errorf("read total: %w: %q must be greater than zero", expense.ErrInvalidAmount, totalStr)
	}

	var fee int64
	if feeStr, ok := valueAfter(lines, "Biaya Admin"); ok {
		if fee, err = parseRupiah(feeStr); err != nil {
			return Transaction{}, fmt.Errorf("read admin fee: %w", err)
		}
	}

	date, err := findDate(lines)
	if err != nil {
		return Transaction{}, err
	}

	kind, _ := valueAfter(lines, "Jenis Transaksi")
	ref, _ := valueAfter(lines, "Nomor Referensi", "No. Ref")
	payee, ok := valueAfter(lines, "Nama Merchant", "Nama Tujuan")
	if !ok {
		// Top ups (DANA) have no merchant line; the kind already says where
		// the money went.
		payee = kind
	}

	return Transaction{
		MessageID: strings.Trim(msg.Header.Get("Message-ID"), "<>"),
		Subject:   msg.Header.Get("Subject"),
		Ref:       ref,
		Kind:      kind,
		Payee:     payee,
		Amount:    amount,
		Fee:       fee,
		Date:      date,
	}, nil
}

// textLines strips the HTML and returns the visible text, one trimmed
// non-empty line per element.
func textLines(htmlBody string) []string {
	s := styleRe.ReplaceAllString(htmlBody, "\n")
	s = tagRe.ReplaceAllString(s, "\n")
	// Turns entities like "&#44;" into ",".
	s = html.UnescapeString(s)

	var lines []string
	for _, l := range strings.Split(s, "\n") {
		// Fields splits on any whitespace run (including non-breaking spaces),
		// so this trims and collapses spaces in one go.
		l = strings.Join(strings.Fields(l), " ")
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

// valueAfter finds the first line equal to one of labels (tried in the order
// given, ignoring case) and returns the line after it. The "..." makes labels
// variadic: callers pass any number of strings and get a slice.
func valueAfter(lines []string, labels ...string) (string, bool) {
	for _, label := range labels {
		for i, l := range lines {
			if strings.EqualFold(l, label) && i+1 < len(lines) {
				return lines[i+1], true
			}
		}
	}
	return "", false
}

// findDate looks for "02 November 2026, 20:15:57 WIB" (or "02 Nov 2026, ...")
// and returns that calendar date. The time of day is ignored.
func findDate(lines []string) (time.Time, error) {
	for _, l := range lines {
		m := dateRe.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		// m[0] is the whole match; m[1..3] are the parenthesised groups.
		day, _ := strconv.Atoi(m[1]) // the regexp guarantees digits
		year, _ := strconv.Atoi(m[3])
		name := strings.ToLower(m[2])
		if len(name) < 3 {
			return time.Time{}, fmt.Errorf("unknown month %q in %q", m[2], l)
		}
		month, ok := months[name[:3]]
		if !ok {
			return time.Time{}, fmt.Errorf("unknown month %q in %q", m[2], l)
		}
		return time.Date(year, month, day, 0, 0, 0, 0, time.UTC), nil
	}
	return time.Time{}, errors.New("transaction date not found in email")
}

// parseRupiah reads BRI amounts: "Rp400.250", "22.000", "Rp0" and
// "Rp1.250.000,00". Dots are thousands separators; a comma starts decimals,
// which must all be zero. Unlike expense.ParseAmount, zero is allowed (fees).
func parseRupiah(s string) (int64, error) {
	orig := s
	s = strings.TrimSpace(s)
	s = strings.TrimSpace(strings.TrimPrefix(s, "Rp"))

	// strings.Cut splits at the first comma; found says whether there was one.
	whole, decimals, found := strings.Cut(s, ",")
	if found && strings.Trim(decimals, "0") != "" {
		return 0, fmt.Errorf("%w: %q has non-zero decimals", expense.ErrInvalidAmount, orig)
	}
	whole = strings.ReplaceAll(whole, ".", "")

	n, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%w: %q is not a rupiah amount", expense.ErrInvalidAmount, orig)
	}
	return n, nil
}
