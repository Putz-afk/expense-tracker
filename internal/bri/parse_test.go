package bri

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Putz-afk/expense-tracker/internal/expense"
)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// The files in testdata are real BRImo emails with personal data replaced.
func TestParse(t *testing.T) {
	tests := []struct {
		name string
		file string
		want Transaction
	}{
		{
			name: "top up DANA (total includes admin fee)",
			file: "testdata/topup_dana.eml",
			want: Transaction{
				MessageID: "1524232943.20461274.1793625413410@GTI-SMSNotifyDB",
				Subject:   "Top Up DANA Berhasil",
				Ref:       "244159045031",
				Kind:      "DANA",
				Payee:     "DANA",
				Amount:    400250,
				Fee:       250,
				Date:      date(2026, time.November, 2),
			},
		},
		{
			name: "QRIS payment (amount has no Rp prefix, fee is Rp0)",
			file: "testdata/qris.eml",
			want: Transaction{
				MessageID: "1688647629.8972004.1793596766347@GTI-SMSNotifyDB",
				Subject:   "Pembelian QRIS Berhasil",
				Ref:       "243791195082",
				Kind:      "QRIS Bayar",
				Payee:     "WARUNG MAKAN ELOK",
				Amount:    22000,
				Fee:       0,
				Date:      date(2026, time.November, 2),
			},
		},
		{
			name: "BI-FAST transfer (Indonesian month name, HTML-escaped comma)",
			file: "testdata/transfer_bifast.eml",
			want: Transaction{
				MessageID: "33605008.20802912.1792858216431@GTI-SMSNotifyDB",
				Subject:   "Pemindahan Dana Bank Lain Dalam Negeri",
				Ref:       "238223860768",
				Kind:      "Transfer BI-FAST",
				Payee:     "BUDI SANTOSO",
				Amount:    57500,
				Fee:       2500,
				Date:      date(2026, time.October, 24),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := os.ReadFile(tc.file)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			got, err := Parse(raw)
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			// time.Time should be compared with Equal, not ==.
			if !got.Date.Equal(tc.want.Date) {
				t.Errorf("Date = %v, want %v", got.Date, tc.want.Date)
			}
			got.Date = tc.want.Date
			if got != tc.want {
				t.Errorf("Parse() =\n%+v\nwant\n%+v", got, tc.want)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	const headers = "From: Bank BRI <BankBRI@bri.co.id>\nSubject: test\nContent-Type: text/html\n\n"

	tests := []struct {
		name    string
		raw     string
		wantErr error // nil means "any error except ErrNotATransaction"
	}{
		{
			name:    "no total at all (e.g. OTP mail)",
			raw:     headers + "<p>Kode OTP Anda 123456</p>",
			wantErr: ErrNotATransaction,
		},
		{
			name: "total is not a number",
			raw:  headers + "<td>Total Transaksi</td><td>abc</td>",
		},
		{
			name: "total is zero",
			raw: headers + "<td>Total Transaksi</td><td>Rp0</td>" +
				"<td>02 Nov 2026, 12:18:14 WIB</td>",
		},
		{
			name: "date missing",
			raw:  headers + "<td>Total Transaksi</td><td>Rp1.000</td>",
		},
		{
			name: "unknown month",
			raw: headers + "<td>Total Transaksi</td><td>Rp1.000</td>" +
				"<td>02 Foo 2026, 12:18:14 WIB</td>",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.raw))
			if err == nil {
				t.Fatal("Parse() returned no error")
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Errorf("error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr == nil && errors.Is(err, ErrNotATransaction) {
				t.Errorf("error = %v, should not be ErrNotATransaction", err)
			}
		})
	}
}

func TestParseRupiah(t *testing.T) {
	tests := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{in: "Rp400.250", want: 400250},
		{in: "22.000", want: 22000},
		{in: "Rp0", want: 0},
		{in: "Rp 1.250.000", want: 1250000},
		{in: "Rp1.250.000,00", want: 1250000},
		{in: "Rp1.250.000,50", wantErr: true},
		{in: "", wantErr: true},
		{in: "Rp", wantErr: true},
		{in: "abc", wantErr: true},
		{in: "-Rp500", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := parseRupiah(tc.in)
			if tc.wantErr {
				if !errors.Is(err, expense.ErrInvalidAmount) {
					t.Fatalf("parseRupiah(%q) error = %v, want ErrInvalidAmount", tc.in, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseRupiah(%q) error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("parseRupiah(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}
