package app

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/macros"
)

var dayNamesExport = [7]string{"Lunedi", "Martedi", "Mercoledi", "Giovedi", "Venerdi", "Sabato", "Domenica"}

func (a *App) handleWeekExport(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	startStr := chi.URLParam(r, "start")
	ws, err := time.Parse("2006-01-02", startStr)
	if err != nil {
		http.Error(w, a.t(r, "week.error.invalid_date"), http.StatusBadRequest)
		return
	}

	plan, err := a.WeeklyPlans.GetOrCreateForWeek(r.Context(), user.ID, ws)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	full, err := a.WeeklyPlans.LoadFull(r.Context(), plan)
	if err != nil {
		a.serverError(w, r, err)
		return
	}

	filename := fmt.Sprintf("nutreek-%s.csv", startStr)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)

	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"giorno", "pasto", "tipo", "nome", "quantita", "unita", "kcal", "proteine", "carbs", "grassi"})

	for d, meals := range full.Days {
		for _, m := range meals {
			items := m.Items
			if m.HasCheats() {
				items = m.Cheats
			}
			for _, it := range items {
				row := exportRow(dayNamesExport[d], m.Name, it)
				_ = cw.Write(row)
			}
		}
	}
	cw.Flush()
}

func exportRow(dayName, mealName string, it domain.MealItemFull) []string {
	m := macros.ForMealItem(it)
	var tipo, nome string
	var qty float64
	var unit string

	if it.Recipe != nil {
		tipo = "ricetta"
		nome = it.Recipe.Name
		qty = it.Quantity
		unit = "porzioni"
	} else if it.Ingredient != nil {
		tipo = "ingrediente"
		nome = it.Ingredient.Name
		qty = it.Quantity
		unit = it.QuantityUnit
	}

	return []string{
		dayName,
		mealName,
		tipo,
		nome,
		strconv.FormatFloat(qty, 'f', 1, 64),
		unit,
		strconv.FormatFloat(m.Kcal, 'f', 1, 64),
		strconv.FormatFloat(m.Protein, 'f', 1, 64),
		strconv.FormatFloat(m.Carbs, 'f', 1, 64),
		strconv.FormatFloat(m.Fat, 'f', 1, 64),
	}
}

func (a *App) handleWeekImport(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	startStr := chi.URLParam(r, "start")
	ws, err := time.Parse("2006-01-02", startStr)
	if err != nil {
		http.Error(w, a.t(r, "week.error.invalid_date"), http.StatusBadRequest)
		return
	}

	if err := r.ParseMultipartForm(2 << 20); err != nil { // 2 MB
		a.Sessions.Put(r.Context(), "flash", "week.flash.file_too_large")
		a.redirectToWeek(w, r, ws)
		return
	}

	f, _, err := r.FormFile("csv_file")
	if err != nil {
		a.Sessions.Put(r.Context(), "flash", "week.flash.select_csv")
		a.redirectToWeek(w, r, ws)
		return
	}
	defer f.Close()

	cr := csv.NewReader(f)
	records, err := cr.ReadAll()
	if err != nil {
		a.Sessions.Put(r.Context(), "flash", "week.flash.invalid_csv")
		a.redirectToWeek(w, r, ws)
		return
	}

	plan, err := a.WeeklyPlans.GetOrCreateForWeek(r.Context(), user.ID, ws)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	full, err := a.WeeklyPlans.LoadFull(r.Context(), plan)
	if err != nil {
		a.serverError(w, r, err)
		return
	}

	// Build a mealName→mealID index per day.
	type mealKey struct {
		day  int
		name string
	}
	mealIndex := make(map[mealKey]int64)
	for d, dayMeals := range full.Days {
		for _, m := range dayMeals {
			k := mealKey{d, strings.ToLower(m.Name)}
			mealIndex[k] = m.ID
		}
	}

	dayIndex := map[string]int{
		"lunedi": 0, "martedi": 1, "mercoledi": 2, "giovedi": 3,
		"venerdi": 4, "sabato": 5, "domenica": 6,
	}

	var notFound []string
	var mealNotFound []string
	imported := 0

	for i, rec := range records {
		if i == 0 {
			continue // header
		}
		if len(rec) < 6 {
			continue
		}
		dayName := strings.ToLower(strings.TrimSpace(rec[0]))
		mealName := strings.TrimSpace(rec[1])
		tipo := strings.TrimSpace(rec[2])
		nome := strings.TrimSpace(rec[3])
		qtyStr := strings.ReplaceAll(strings.TrimSpace(rec[4]), ",", ".")

		di, ok := dayIndex[dayName]
		if !ok || mealName == "" || nome == "" {
			continue
		}
		if tipo != "ingrediente" {
			continue
		}

		qty, err := strconv.ParseFloat(qtyStr, 64)
		if err != nil || qty <= 0 {
			continue
		}

		mealID, ok := mealIndex[mealKey{di, strings.ToLower(mealName)}]
		if !ok {
			key := dayName + "/" + mealName
			if !containsStr(mealNotFound, key) {
				mealNotFound = append(mealNotFound, key)
			}
			continue
		}

		ings, err := a.Ingredients.SearchActive(r.Context(), user.ID, nome, 5)
		if err != nil || len(ings) == 0 {
			if !containsStr(notFound, nome) {
				notFound = append(notFound, nome)
			}
			continue
		}
		var ing *domain.Ingredient
		for _, candidate := range ings {
			if strings.EqualFold(candidate.Name, nome) {
				ing = candidate
				break
			}
		}
		if ing == nil {
			if !containsStr(notFound, nome) {
				notFound = append(notFound, nome)
			}
			continue
		}

		if _, err := a.WeeklyPlans.AddItem(r.Context(), mealID, user.ID, nil, &ing.ID, qty, "g"); err != nil {
			continue
		}
		imported++
	}

	parts := []string{a.t(r, "week.flash.imported", "Count", imported)}
	if len(mealNotFound) > 0 {
		parts = append(parts, a.t(r, "week.flash.meals_not_found", "List", strings.Join(mealNotFound, ", ")))
	}
	if len(notFound) > 0 {
		parts = append(parts, a.t(r, "week.flash.ingredients_not_found", "List", strings.Join(notFound, ", ")))
	}
	msg := strings.Join(parts, " ")
	a.Sessions.Put(r.Context(), "flash", msg)
	a.redirectToWeek(w, r, ws)
}

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
