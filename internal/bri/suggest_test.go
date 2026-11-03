package bri

import "testing"

func TestSuggestCategory(t *testing.T) {
	tests := []struct {
		name string
		tx   Transaction
		want string
	}{
		{"food stall", Transaction{Kind: "QRIS Bayar", Payee: "WARUNG MAKAN ELOK"}, "food"},
		{"coffee", Transaction{Kind: "QRIS Bayar", Payee: "Kopi Kenangan - Senayan"}, "food"},
		{"minimarket", Transaction{Kind: "QRIS Bayar", Payee: "INDOMARET 12345"}, "shopping"},
		{"ride hailing", Transaction{Kind: "QRIS Bayar", Payee: "GOJEK INDONESIA"}, "transport"},
		{"fuel", Transaction{Kind: "QRIS Bayar", Payee: "SPBU 31.123.45"}, "transport"},
		{"pharmacy", Transaction{Kind: "QRIS Bayar", Payee: "Apotek Sehat"}, "health"},
		{"cinema", Transaction{Kind: "QRIS Bayar", Payee: "CGV Grand Indonesia"}, "entertainment"},
		{"utility", Transaction{Kind: "QRIS Bayar", Payee: "PLN Prabayar"}, "bills"},
		{"unknown merchant", Transaction{Kind: "QRIS Bayar", Payee: "PT ABC SEJAHTERA"}, "other"},
		{"whole words only: 'tol' is not inside 'stolen'", Transaction{Kind: "QRIS Bayar", Payee: "STOLEN GOODS"}, "other"},
		{"kind is matched ignoring case", Transaction{Kind: "qris bayar", Payee: "warung makan"}, "food"},
		{"top up ignores payee", Transaction{Kind: "DANA", Payee: "DANA"}, "other"},
		{"transfer ignores payee, even a food-like name", Transaction{Kind: "Transfer BI-FAST", Payee: "KOPI BUDI"}, "other"},
		{"empty", Transaction{}, "other"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := SuggestCategory(tc.tx); got != tc.want {
				t.Errorf("SuggestCategory(%+v) = %q, want %q", tc.tx, got, tc.want)
			}
		})
	}
}
