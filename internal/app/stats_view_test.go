package app

import (
	"testing"
	"time"

	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/macros"
	"github.com/amon2126/nutreek/internal/service"
	"github.com/amon2126/nutreek/internal/tdee"
)

func statsDay(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func statsFixture() statsInput {
	w1, w2, w3 := statsDay(2026, 9, 28), statsDay(2026, 10, 5), statsDay(2026, 10, 12)
	return statsInput{
		Weeks: []statsWeek{
			{Start: w1, Total: macros.Macros{Kcal: 14000, Protein: 840, Carbs: 1610, Fat: 455}, DaysLogged: 7,
				WorkoutKcal: 900, Workouts: 3, Cheats: 1, WaterML: 14000, Adherence: 90, Planned: true},
			{Start: w2}, // settimana vuota
			{Start: w3, Total: macros.Macros{Kcal: 9000, Protein: 600, Carbs: 1000, Fat: 300}, DaysLogged: 5,
				WorkoutKcal: 300, Workouts: 1, Cheats: 2, Adherence: 80, Planned: true},
		},
		Weights: []domain.WeightLog{
			{MeasuredAt: statsDay(2026, 9, 1), WeightKg: 80}, // fuori finestra
			{MeasuredAt: statsDay(2026, 9, 28), WeightKg: 74},
			{MeasuredAt: statsDay(2026, 10, 15), WeightKg: 72.4},
		},
		TDEE: &service.TDEEReport{HasEnoughData: true, WeeklyData: []tdee.WeekData{
			{WeekStart: w1, AvgKcal: 2000, StartWeight: 74, EndWeight: 73.8},
			{WeekStart: w3, AvgKcal: 1800, StartWeight: 73, EndWeight: 72.9},
		}},
		TargetKcal:    2100,
		WaterTargetML: 2000,
		StreakDays:    6,
	}
}

func TestBuildStatsViewTiles(t *testing.T) {
	v := buildStatsView(statsFixture())

	// 23.000 kcal su 12 giorni registrati.
	if v.AvgDayKcal < 1916 || v.AvgDayKcal > 1917 {
		t.Errorf("media al giorno = %.1f, attesa 1916,7", v.AvgDayKcal)
	}
	want := []statsTile{
		{Label: "Media al giorno", Value: "1.917", Unit: "kcal", Sub: "obiettivo 2.100"},
		{Label: "Peso", Value: "72,4", Unit: "kg", Sub: "−1,6 kg in 2 settimane"},
		{Label: "Allenamenti", Value: "2", Unit: "a settimana", Sub: "600 kcal in media"},
		{Label: "Sgarri", Value: "1,5", Unit: "a settimana", Sub: "6 giorni senza"},
		{Label: "Acqua", Value: "2", Unit: "l al giorno", Sub: "obiettivo 2 l"},
	}
	if len(v.Tiles) != len(want) {
		t.Fatalf("tile = %+v", v.Tiles)
	}
	for i := range want {
		if v.Tiles[i] != want[i] {
			t.Errorf("tile %d = %+v, attesa %+v", i, v.Tiles[i], want[i])
		}
	}
}

func TestBuildStatsViewRowsAndCharts(t *testing.T) {
	v := buildStatsView(statsFixture())

	// La tabella parte dalla settimana piu' recente, con medie al giorno.
	if len(v.Rows) != 3 || !v.Rows[0].Start.Equal(statsDay(2026, 10, 12)) {
		t.Fatalf("righe = %+v", v.Rows)
	}
	if r := v.Rows[0]; r.PerDay.Kcal != 1800 || r.PerDay.Protein != 120 || !r.HasAdherence || r.Adherence != 80 {
		t.Errorf("riga recente = %+v", r)
	}
	if r := v.Rows[1]; r.DaysLogged != 0 || r.HasAdherence {
		t.Errorf("settimana vuota = %+v", r)
	}

	c := v.Charts
	if got := c.Labels; len(got) != 3 || got[0] != "28 set" || got[2] != "12 ott" {
		t.Errorf("etichette = %v", got)
	}
	// Barre impilate: kcal al giorno da proteine (×4), carboidrati (×4), grassi (×9).
	if c.Prot[0] != 480 || c.Carb[0] != 920 || c.Fat[0] != 585 || c.Prot[1] != 0 {
		t.Errorf("macro = %v %v %v", c.Prot, c.Carb, c.Fat)
	}
	if c.TDEE[0] == nil || c.TDEE[1] != nil || c.TDEE[2] == nil {
		t.Errorf("fabbisogno = %v, attesi valori solo nelle settimane stimate", c.TDEE)
	}
	if c.Adherence[1] != nil || *c.Adherence[2] != 80 {
		t.Errorf("aderenza = %v", c.Adherence)
	}
	if c.Water[0] != 2 || c.Water[2] != 0 {
		t.Errorf("acqua = %v", c.Water)
	}
	// Il peso si disegna solo nella finestra delle settimane mostrate.
	if len(c.Weight) != 2 || c.WeightLabels[0] != "28 set" {
		t.Errorf("peso = %v %v", c.WeightLabels, c.Weight)
	}
	if !v.HasMacro || !v.HasWeight || !v.HasTDEEChart || !v.HasWorkout || !v.HasCheats || !v.HasWater || !v.HasAdherence {
		t.Errorf("flag = %+v", v)
	}
}

func TestBuildStatsViewEmpty(t *testing.T) {
	v := buildStatsView(statsInput{Weeks: []statsWeek{{Start: statsDay(2026, 10, 12)}}})
	if v.HasMacro || v.HasWeight || v.HasTDEEChart {
		t.Errorf("senza dati nessun grafico: %+v", v)
	}
	if v.Tiles[0].Value != "—" || v.Tiles[1].Sub != "nessuna pesata" {
		t.Errorf("tile vuote = %+v", v.Tiles)
	}
}
