package bri

import (
	"strings"
	"unicode"
)

// defaultCategory is used when nothing matches. It is one of the categories
// the database starts with.
const defaultCategory = "other"

// A slice of structs, not a map: ranging over a Go map visits keys in random
// order, so with a map a payee matching two categories would get a different
// answer from run to run. A slice is checked top to bottom, every time.
var categoryKeywords = []struct {
	category string
	keywords []string // lowercase; a keyword may be several words
}{
	{"transport", []string{
		"gojek", "grab", "gocar", "goride", "maxim", "bluebird", "parkir", "tol",
		"spbu", "pertamina", "shell", "krl", "mrt", "lrt", "transjakarta", "kai",
	}},
	{"health", []string{
		"apotek", "farmasi", "klinik", "rumah sakit", "kimia farma", "guardian",
	}},
	{"entertainment", []string{
		"cgv", "xxi", "cinema", "netflix", "spotify", "steam", "bioskop",
	}},
	{"bills", []string{
		"pln", "pdam", "telkom", "indihome", "bpjs", "telkomsel", "xl", "pajak",
	}},
	{"food", []string{
		"warung", "makan", "resto", "restoran", "cafe", "kafe", "kopi", "bakso",
		"nasi", "mie", "ayam", "sate", "bakery", "roti", "kedai", "depot",
		"geprek", "soto", "martabak", "starbucks", "mcdonalds", "kfc",
	}},
	{"shopping", []string{
		"indomaret", "alfamart", "alfamidi", "tokopedia", "shopee", "toko",
		"supermarket", "hypermart", "lotte", "mart",
	}},
}

// SuggestCategory guesses a category for t. Only QRIS payments are guessed
// from the merchant name; transfers and top ups say nothing about what the
// money was for, so they get "other" and the user decides while reviewing.
func SuggestCategory(t Transaction) string {
	if !strings.Contains(strings.ToLower(t.Kind), "qris") {
		return defaultCategory
	}

	// Pad with spaces on both sides so " tol " matches the word "tol" but not
	// the inside of "stolen". Multi-word keywords like "rumah sakit" work too.
	padded := " " + normalizeWords(t.Payee) + " "
	for _, group := range categoryKeywords {
		for _, kw := range group.keywords {
			if strings.Contains(padded, " "+kw+" ") {
				return group.category
			}
		}
	}
	return defaultCategory
}

// normalizeWords lowercases s and replaces every run of non-letters and
// non-digits with one space: "Kopi Kenangan - Senayan" -> "kopi kenangan senayan".
func normalizeWords(s string) string {
	// strings.FieldsFunc splits wherever the function returns true.
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	return strings.Join(words, " ")
}
