package expense

import (
	"errors"
	"testing"
)

func TestParseAmount(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{"simple", "25000", 25000, false},
		{"one", "1", 1, false},
		{"surrounding spaces", "  500 ", 500, false},
		{"large", "9000000000", 9000000000, false},
		{"empty", "", 0, true},
		{"only spaces", "   ", 0, true},
		{"zero", "0", 0, true},
		{"negative", "-5", 0, true},
		{"decimal point", "12.50", 0, true},
		{"thousands comma", "25,000", 0, true},
		{"letters", "abc", 0, true},
		{"mixed", "12abc", 0, true},
		{"overflow", "99999999999999999999", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseAmount(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseAmount(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, ErrInvalidAmount) {
				t.Errorf("ParseAmount(%q) error %v does not wrap ErrInvalidAmount", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseAmount(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatAmount(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "0"},
		{5, "5"},
		{999, "999"},
		{1000, "1,000"},
		{25000, "25,000"},
		{150000, "150,000"},
		{1234567, "1,234,567"},
		{-1500, "-1,500"},
	}

	for _, tt := range tests {
		if got := FormatAmount(tt.in); got != tt.want {
			t.Errorf("FormatAmount(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
