package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/amon2126/nutreek/internal/i18n"
)

func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestFormatNum(t *testing.T) {
	cases := []struct {
		v      float64
		it, en string
	}{
		{0, "0", "0"},
		{999.4, "999", "999"},
		{1636.4, "1.636", "1,636"},
		{1894, "1.894", "1,894"},
		{12345, "12.345", "12,345"},
		{1234567, "1.234.567", "1,234,567"},
		{-1500, "-1.500", "-1,500"},
		{-0.4, "0", "0"},
	}
	for _, tc := range cases {
		if got := formatNum("it", tc.v); got != tc.it {
			t.Errorf("formatNum(it, %v) = %q, want %q", tc.v, got, tc.it)
		}
		if got := formatNum("en", tc.v); got != tc.en {
			t.Errorf("formatNum(en, %v) = %q, want %q", tc.v, got, tc.en)
		}
	}
}

func TestFormatDec(t *testing.T) {
	cases := []struct {
		v      float64
		digits int
		it, en string
	}{
		{1.5, 1, "1,5", "1.5"},
		{13.5, 1, "13,5", "13.5"},
		{2, 1, "2", "2"},
		{1.25, 2, "1,25", "1.25"},
		{0.5, 2, "0,5", "0.5"},
		{0, 2, "0", "0"},
		{-1.5, 1, "-1,5", "-1.5"},
		{1894.5, 1, "1894,5", "1894.5"}, // decimals are not grouped
	}
	for _, tc := range cases {
		if got := formatDec("it", tc.v, tc.digits); got != tc.it {
			t.Errorf("formatDec(it, %v, %d) = %q, want %q", tc.v, tc.digits, got, tc.it)
		}
		if got := formatDec("en", tc.v, tc.digits); got != tc.en {
			t.Errorf("formatDec(en, %v, %d) = %q, want %q", tc.v, tc.digits, got, tc.en)
		}
	}
}

// The exported helpers are what handlers use for HTML written by hand.
func TestExportedFormatters(t *testing.T) {
	if got := FormatNum("en", 1894); got != "1,894" {
		t.Errorf("FormatNum(en) = %q", got)
	}
	if got := FormatNum("it", 1894); got != "1.894" {
		t.Errorf("FormatNum(it) = %q", got)
	}
	if got := FormatDec("en", 13.5, 1); got != "13.5" {
		t.Errorf("FormatDec(en) = %q", got)
	}
	if got := FormatDec("it", 13.5, 1); got != "13,5" {
		t.Errorf("FormatDec(it) = %q", got)
	}
	// An unknown or empty language behaves like the default (Italian).
	if got := FormatNum("", 1894); got != "1.894" {
		t.Errorf("FormatNum(\"\") = %q", got)
	}
}

func TestDayNames(t *testing.T) {
	cases := []struct {
		d                int
		itFull, enFull   string
		itShort, enShort string
	}{
		{-1, "Ogni giorno", "Every day", "Ogni giorno", "Every day"},
		{0, "Lunedì", "Monday", "Lun", "Mon"},
		{2, "Mercoledì", "Wednesday", "Mer", "Wed"},
		{6, "Domenica", "Sunday", "Dom", "Sun"},
		{7, "?", "?", "?", "?"},
	}
	for _, tc := range cases {
		if got := DayName("it", tc.d); got != tc.itFull {
			t.Errorf("DayName(it, %d) = %q, want %q", tc.d, got, tc.itFull)
		}
		if got := DayName("en", tc.d); got != tc.enFull {
			t.Errorf("DayName(en, %d) = %q, want %q", tc.d, got, tc.enFull)
		}
		if got := dayLabel("it", tc.d); got != tc.itShort {
			t.Errorf("dayLabel(it, %d) = %q, want %q", tc.d, got, tc.itShort)
		}
		if got := dayLabel("en", tc.d); got != tc.enShort {
			t.Errorf("dayLabel(en, %d) = %q, want %q", tc.d, got, tc.enShort)
		}
	}
}

func TestDayHelpers(t *testing.T) {
	f := funcMap("it")
	g := funcMap("en")
	// 2026-10-12 is a Monday, 2026-10-18 a Sunday: the week starts on Monday.
	mon, sun := date(2026, 10, 12), date(2026, 10, 18)
	cases := []struct {
		name   string
		day    time.Time
		it, en string
	}{
		{"dayTitle", mon, "Lunedì 12", "Monday 12"},
		{"dayTitle", sun, "Domenica 18", "Sunday 18"},
		{"dayShort", mon, "Lun 12", "Mon 12"},
		{"dayShort", sun, "Dom 18", "Sun 18"},
		{"dayLetter", mon, "L", "M"},
		{"dayLetter", sun, "D", "S"},
		{"monthName", mon, "ottobre", "October"},
		{"monthName", date(2026, 1, 1), "gennaio", "January"},
	}
	for _, tc := range cases {
		if got := f[tc.name].(func(time.Time) string)(tc.day); got != tc.it {
			t.Errorf("%s(it, %v) = %q, want %q", tc.name, tc.day.Format("2006-01-02"), got, tc.it)
		}
		if got := g[tc.name].(func(time.Time) string)(tc.day); got != tc.en {
			t.Errorf("%s(en, %v) = %q, want %q", tc.name, tc.day.Format("2006-01-02"), got, tc.en)
		}
	}
}

func TestShortDate(t *testing.T) {
	if got := ShortDate("it", date(2026, 10, 12)); got != "12 ott" {
		t.Errorf("it = %q", got)
	}
	if got := ShortDate("en", date(2026, 10, 12)); got != "Oct 12" {
		t.Errorf("en = %q", got)
	}
	if got := ShortDate("en", date(2026, 5, 3)); got != "May 3" {
		t.Errorf("en May = %q", got)
	}
}

func TestLiters(t *testing.T) {
	if got := funcMap("it")["liters"].(func(int) string)(1500); got != "1,5 l" {
		t.Errorf("it = %q", got)
	}
	if got := funcMap("en")["liters"].(func(int) string)(1500); got != "1.5 l" {
		t.Errorf("en = %q", got)
	}
}

func TestWeekRange(t *testing.T) {
	cases := []struct {
		name   string
		start  time.Time
		it, en string
	}{
		{"same month", date(2026, 10, 12), "12–18 ottobre", "October 12–18"},
		{"month crossing", date(2026, 9, 28), "28 settembre – 4 ottobre", "September 28 – October 4"},
		{"year crossing", date(2025, 12, 29), "29 dicembre 2025 – 4 gennaio 2026", "December 29, 2025 – January 4, 2026"},
	}
	for _, tc := range cases {
		if got := weekRange("it", tc.start); got != tc.it {
			t.Errorf("%s: weekRange(it) = %q, want %q", tc.name, got, tc.it)
		}
		if got := weekRange("en", tc.start); got != tc.en {
			t.Errorf("%s: weekRange(en) = %q, want %q", tc.name, got, tc.en)
		}
	}
}

// The shopping list can cover several weeks: the range reaches the Sunday of
// the last one.
func TestWeeksRange(t *testing.T) {
	start := date(2026, 10, 12)
	cases := []struct {
		n      int
		it, en string
	}{
		{1, "12–18 ottobre", "October 12–18"},
		{3, "12 ottobre – 1 novembre", "October 12 – November 1"},
		{12, "12 ottobre 2026 – 3 gennaio 2027", "October 12, 2026 – January 3, 2027"},
	}
	for _, tc := range cases {
		if got := weeksRange("it", start, tc.n); got != tc.it {
			t.Errorf("%d weeks (it) = %q, want %q", tc.n, got, tc.it)
		}
		if got := weeksRange("en", start, tc.n); got != tc.en {
			t.Errorf("%d weeks (en) = %q, want %q", tc.n, got, tc.en)
		}
	}
}

// Templates get the formatters bound to the request's language.
func TestTemplatesFormatByLanguage(t *testing.T) {
	tfs := fstest.MapFS{
		"templates/layouts/base.html": {Data: []byte(`{{define "layout"}}<html lang="{{lang}}">{{template "body" .}}</html>{{end}}`)},
		"templates/pages/probe.html": {Data: []byte(`{{define "body"}}` +
			`{{num 1894}}|{{dec 13.5 1}}|{{liters 1500}}|{{dayTitle .Day}}|{{dayLabel 0}}|{{weekRange .Day}}` +
			`{{end}}`)},
	}
	r, err := NewRenderer(tfs)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ lang, want string }{
		{"it", `<html lang="it">1.894|13,5|1,5 l|Lunedì 12|Lun|12–18 ottobre</html>`},
		{"en", `<html lang="en">1,894|13.5|1.5 l|Monday 12|Mon|October 12–18</html>`},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req = req.WithContext(i18n.WithLang(req.Context(), tc.lang))
		rec := httptest.NewRecorder()
		if err := r.Render(rec, req, http.StatusOK, "probe", map[string]any{"Day": date(2026, 10, 12)}); err != nil {
			t.Fatal(err)
		}
		if got := strings.TrimSpace(rec.Body.String()); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.lang, got, tc.want)
		}
	}
}
