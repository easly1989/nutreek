package web

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/amon2126/nutreek/internal/i18n"
)

// Number and date formatting follows the page language (#157). Day and month
// names live in the "format" catalog; the short forms (Lun, Mon, ott, Oct) are
// the first three letters of the full name, the day letter the first one.

var weekdayKeys = [7]string{ // Monday first: the week starts on Monday in every language
	"format.weekday.monday", "format.weekday.tuesday", "format.weekday.wednesday",
	"format.weekday.thursday", "format.weekday.friday", "format.weekday.saturday",
	"format.weekday.sunday",
}

var monthKeys = [12]string{
	"format.month.january", "format.month.february", "format.month.march",
	"format.month.april", "format.month.may", "format.month.june",
	"format.month.july", "format.month.august", "format.month.september",
	"format.month.october", "format.month.november", "format.month.december",
}

// separators returns the thousands and decimal separators of lang.
func separators(lang string) (thousands, decimal string) {
	if lang == "en" {
		return ",", "."
	}
	return ".", ","
}

// prefix returns the first n letters of s.
func prefix(s string, n int) string {
	r := []rune(s)
	return string(r[:min(n, len(r))])
}

// weekdayIndex maps time.Weekday (Sunday 0) to the Monday-first index.
func weekdayIndex(t time.Time) int { return (int(t.Weekday()) + 6) % 7 }

func weekdayName(lang string, t time.Time) string {
	return i18n.T(lang, weekdayKeys[weekdayIndex(t)])
}

func monthNameOf(lang string, t time.Time) string {
	return i18n.T(lang, monthKeys[t.Month()-1])
}

// DayName is the full name of a plan day: 0 is Monday, 6 Sunday, -1 "every
// day" (the day of a template slot that repeats daily).
func DayName(lang string, d int) string {
	switch {
	case d == -1:
		return i18n.T(lang, "format.every_day")
	case d >= 0 && d < 7:
		return i18n.T(lang, weekdayKeys[d])
	}
	return "?"
}

// dayLabel is the short name of a plan day ("Lun", "Mon"); -1 is "every day".
func dayLabel(lang string, d int) string {
	if d == -1 {
		return DayName(lang, d)
	}
	if d >= 0 && d < 7 {
		return prefix(DayName(lang, d), 3)
	}
	return "?"
}

// ShortDate writes a date as on the chart axes: "12 ott", "Oct 12".
func ShortDate(lang string, t time.Time) string {
	return i18n.T(lang, "format.short_date", "Day", t.Day(), "Month", prefix(monthNameOf(lang, t), 3))
}

// FormatNum and FormatDec expose the templates' formatting to the HTML
// fragments and view models the handlers build by hand.
func FormatNum(lang string, v float64) string             { return formatNum(lang, v) }
func FormatDec(lang string, v float64, digits int) string { return formatDec(lang, v, digits) }

// formatNum rounds to the integer and groups thousands the way lang writes
// them: 1636.4 -> "1.636" (it), "1,636" (en).
func formatNum(lang string, v float64) string {
	thousands, _ := separators(lang)
	n := int64(math.Round(v))
	sign := ""
	if n < 0 {
		sign, n = "-", -n
	}
	s := strconv.FormatInt(n, 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + thousands + s[i:]
	}
	return sign + s
}

// formatDec writes v with at most `digits` decimals, the language's decimal
// separator and no trailing zeros: 1.5 -> "1,5" (it), "1.5" (en); 2.0 -> "2".
func formatDec(lang string, v float64, digits int) string {
	_, decimal := separators(lang)
	s := strconv.FormatFloat(v, 'f', digits, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return strings.Replace(s, ".", decimal, 1)
}

// weekRange describes the week starting on Monday start:
// "12–18 ottobre" / "October 12–18", "29 settembre – 5 ottobre" /
// "September 29 – October 5", "29 dicembre 2025 – 4 gennaio 2026" /
// "December 29, 2025 – January 4, 2026".
func weekRange(lang string, start time.Time) string { return weeksRange(lang, start, 1) }

// weeksRange describes n consecutive weeks from Monday start to the Sunday
// of the last one: "12 ottobre – 1 novembre" / "October 12 – November 1".
func weeksRange(lang string, start time.Time, n int) string {
	end := start.AddDate(0, 0, 7*max(n, 1)-1)
	args := []any{
		"StartDay", start.Day(), "StartMonth", monthNameOf(lang, start), "StartYear", start.Year(),
		"EndDay", end.Day(), "EndMonth", monthNameOf(lang, end), "EndYear", end.Year(),
	}
	switch {
	case start.Year() != end.Year():
		return i18n.T(lang, "format.range.cross_year", args...)
	case start.Month() != end.Month():
		return i18n.T(lang, "format.range.cross_month", args...)
	default:
		return i18n.T(lang, "format.range.same_month", args...)
	}
}
