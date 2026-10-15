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
		{"giorno richiesto in settimana", "2026-10-17", thu, "2026-10-17"},
		{"senza parametro: oggi", "", thu, "2026-10-15"},
		{"parametro fuori settimana: oggi", "2026-10-25", thu, "2026-10-15"},
		{"parametro non valido: oggi", "domani", thu, "2026-10-15"},
		{"oggi fuori settimana: lunedi'", "", nextWeek, "2026-10-12"},
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
