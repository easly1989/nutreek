package app

import (
	"testing"
	"time"
)

func TestSelectedDay(t *testing.T) {
	mon := time.Date(2026, 10, 12, 0, 0, 0, 0, time.Local)
	thu := time.Date(2026, 10, 15, 0, 0, 0, 0, time.Local)
	nextWeek := time.Date(2026, 10, 21, 0, 0, 0, 0, time.Local)
	cases := []struct {
		name, param string
		today       time.Time
		want        string
	}{
		{"requested day in the week", "2026-10-17", thu, "2026-10-17"},
		{"no parameter: today", "", thu, "2026-10-15"},
		{"parameter outside the week: today", "2026-10-25", thu, "2026-10-15"},
		{"invalid parameter: today", "domani", thu, "2026-10-15"},
		{"today outside the week: Monday", "", nextWeek, "2026-10-12"},
		{"domenica compresa", "2026-10-18", nextWeek, "2026-10-18"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatDate(selectedDay(tc.param, mon, tc.today)); got != tc.want {
				t.Errorf("got %s want %s", got, tc.want)
			}
		})
	}
}
