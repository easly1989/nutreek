package app

import (
	"strings"
	"unicode/utf8"
)

// MaxNameLen is the standard length limit for "name" fields (recipe,
// template, slot, meal). Deliberately generous so as not to frustrate the
// user, but small enough that malicious input cannot bloat the DB.
const MaxNameLen = 200

// MaxDescriptionLen applies to the long description fields (recipe.description).
const MaxDescriptionLen = 2000

// MaxItemQuantity caps the quantity of a recipe row or a meal item (grams,
// pieces or servings): anything above it is a typo.
const MaxItemQuantity = 100_000

// parseItemQuantity reads the quantity of a recipe row or a meal item: a
// number above 0 and at most MaxItemQuantity, «,» or «.» as the decimal mark.
func parseItemQuantity(s string) (float64, bool) {
	v, err := parseNonNegFloat(strings.TrimSpace(s))
	return v, err == nil && v > 0 && v <= MaxItemQuantity
}

// withinLen returns true when the string s has at most max RUNES (not bytes:
// Italian UTF-8 can have 2-byte characters and counting them as 2 would be
// wrong). An empty string is always fine as far as the limit goes.
func withinLen(s string, max int) bool {
	return utf8.RuneCountInString(s) <= max
}

// truncateRunes cuts s to max runes, respecting UTF-8 boundaries. Used to
// normalize imported fields (e.g. from a web page) within the form limits.
func truncateRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max])
}
