package app

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseWeightRange(t *testing.T) {
	cases := map[string]int{"": 90, "?giorni=30": 30, "?giorni=365": 365, "?giorni=7": 90, "?giorni=abc": 90}
	for q, want := range cases {
		if got := parseWeightRange(httptest.NewRequest("GET", "/profile/peso"+q, nil)); got != want {
			t.Errorf("%q: %d days, want %d", q, got, want)
		}
	}
}

func TestWeightTrend(t *testing.T) {
	day := 24 * time.Hour
	cases := []struct {
		delta float64
		span  time.Duration
		want  string
	}{
		{-0.8, 30 * day, "−0,8 kg in 4 settimane"},
		{0.4, 5 * day, "+0,4 kg in 5 giorni"},
		{0.02, 10 * day, "stabile"},
		{-0.3, 0, "−0,3 kg"},
	}
	for _, c := range cases {
		if got := weightTrend("it", c.delta, c.span); got != c.want {
			t.Errorf("weightTrend(%v, %v) = %q, want %q", c.delta, c.span, got, c.want)
		}
	}
}
