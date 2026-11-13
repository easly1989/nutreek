package db

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// maxOFFNameLen caps a normalised name, in runes (the app's MaxNameLen).
const maxOFFNameLen = 200

// NormalizeOFFName applies the rules used when an Open Food Facts product is
// saved as a local ingredient (see docs/off-strategy.md, "Imported names"):
//
//  1. collapse every run of whitespace into one space and trim the ends;
//  2. strip trailing separators and punctuation (",", ";", ":", "-", ".");
//  3. a name written entirely in capitals (at least 4 letters) is lowercased,
//     so "YOGURT GRECO 0%" and "Yogurt greco 0%" are the same ingredient;
//  4. the first letter is uppercased, the rest is left as OFF has it;
//  5. cut to 200 runes.
func NormalizeOFFName(name string) string {
	name = strings.Join(strings.Fields(name), " ")
	name = strings.TrimRight(name, " ,;:-.")
	if isAllCaps(name) {
		name = strings.ToLower(name)
	}
	if r, size := utf8.DecodeRuneInString(name); size > 0 {
		name = string(unicode.ToUpper(r)) + name[size:]
	}
	if utf8.RuneCountInString(name) > maxOFFNameLen {
		name = string([]rune(name)[:maxOFFNameLen])
	}
	return name
}

// NormalizeOFFBrand collapses whitespace and trims a brand; for several
// brands separated by commas it keeps the first one. It returns nil when
// nothing is left.
func NormalizeOFFBrand(brand *string) *string {
	if brand == nil {
		return nil
	}
	b, _, _ := strings.Cut(*brand, ",")
	b = strings.Join(strings.Fields(b), " ")
	if b == "" {
		return nil
	}
	return &b
}

// isAllCaps reports whether s has at least 4 letters and none is lowercase.
func isAllCaps(s string) bool {
	letters := 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			letters++
			if unicode.IsLower(r) {
				return false
			}
		}
	}
	return letters >= 4
}
