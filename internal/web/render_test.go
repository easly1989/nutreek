package web

import (
	"testing"
	"time"
)

func TestNavActive(t *testing.T) {
	cases := []struct {
		cur, href string
		want      bool
	}{
		{"/", "/", true},
		{"/recipes", "/", false},
		{"/recipes", "/recipes", true},
		{"/recipes/12/edit", "/recipes", true},
		{"/recipes-old", "/recipes", false},
		{"/profile/peso", "/profile/peso", true},
		{"/stats", "/shopping-list", false},
	}
	for _, tc := range cases {
		if got := navActive(tc.cur, tc.href); got != tc.want {
			t.Errorf("navActive(%q, %q) = %v, want %v", tc.cur, tc.href, got, tc.want)
		}
	}
}

func TestFormatNum(t *testing.T) {
	cases := map[float64]string{0: "0", 999.4: "999", 1636.4: "1.636", 12345: "12.345", 1234567: "1.234.567", -1500: "-1.500"}
	for in, want := range cases {
		if got := formatNum(in); got != want {
			t.Errorf("formatNum(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatDec(t *testing.T) {
	cases := []struct {
		v      float64
		digits int
		want   string
	}{{1.5, 1, "1,5"}, {2, 1, "2"}, {1.25, 2, "1,25"}, {0.5, 2, "0,5"}, {0, 2, "0"}}
	for _, tc := range cases {
		if got := formatDec(tc.v, tc.digits); got != tc.want {
			t.Errorf("formatDec(%v, %d) = %q, want %q", tc.v, tc.digits, got, tc.want)
		}
	}
}

func TestMonthName(t *testing.T) {
	monthName := funcMap()["monthName"].(func(time.Time) string)
	if got := monthName(time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)); got != "ottobre" {
		t.Errorf("monthName(15/10) = %q", got)
	}
	if got := monthName(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); got != "gennaio" {
		t.Errorf("monthName(1/1) = %q", got)
	}
}

func TestWeekRange(t *testing.T) {
	cases := []struct {
		start time.Time
		want  string
	}{
		{time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC), "12–18 ottobre"},
		{time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC), "28 settembre – 4 ottobre"},
		{time.Date(2025, 12, 29, 0, 0, 0, 0, time.UTC), "29 dicembre 2025 – 4 gennaio 2026"},
	}
	for _, tc := range cases {
		if got := weekRange(tc.start); got != tc.want {
			t.Errorf("weekRange(%v) = %q, want %q", tc.start, got, tc.want)
		}
	}
}

// The shopping list can cover several weeks: the range reaches the Sunday of
// the last one.
func TestWeeksRange(t *testing.T) {
	start := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	if got := weeksRange(start, 1); got != "12–18 ottobre" {
		t.Errorf("1 week = %q", got)
	}
	if got := weeksRange(start, 3); got != "12 ottobre – 1 novembre" {
		t.Errorf("3 settimane = %q", got)
	}
}
