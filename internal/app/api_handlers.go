package app

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/macros"
)

// ---- JSON response types ----------------------------------------------------

type apiUserResponse struct {
	ID          int64   `json:"id"`
	Email       string  `json:"email"`
	DisplayName string  `json:"display_name"`
	Role        string  `json:"role"`
	WeightKg    float64 `json:"weight_kg,omitempty"`
	HeightCm    float64 `json:"height_cm,omitempty"`
}

type apiDayMacros struct {
	Date    string  `json:"date"`
	Kcal    float64 `json:"kcal"`
	Protein float64 `json:"protein_g"`
	Carbs   float64 `json:"carbs_g"`
	Fat     float64 `json:"fat_g"`
}

type apiWeekResponse struct {
	WeekStart string         `json:"week_start"`
	Days      []apiDayMacros `json:"days"`
}

type apiIngredientResponse struct {
	ID            int64   `json:"id"`
	Name          string  `json:"name"`
	Brand         string  `json:"brand,omitempty"`
	Unit          string  `json:"unit"`
	KcalPer100    float64 `json:"kcal_per_100"`
	ProteinPer100 float64 `json:"protein_per_100"`
	CarbsPer100   float64 `json:"carbs_per_100"`
	FatPer100     float64 `json:"fat_per_100"`
}

// ---- helper -----------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ---- handler GET /api/v1/me -------------------------------------------------

func (a *App) handleAPIMe(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	resp := apiUserResponse{
		ID:          user.ID,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		Role:        user.Role,
	}
	if user.WeightKg != nil {
		resp.WeightKg = *user.WeightKg
	}
	if user.HeightCm != nil {
		resp.HeightCm = *user.HeightCm
	}
	writeJSON(w, http.StatusOK, resp)
}

// ---- handler GET /api/v1/week -----------------------------------------------
// Query params: ?start=YYYY-MM-DD (default: the current week)

func (a *App) handleAPIWeek(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	ctx := r.Context()

	var weekStart time.Time
	if s := r.URL.Query().Get("start"); s != "" {
		parsed, err := time.Parse("2006-01-02", s)
		if err != nil {
			writeJSONErr(w, http.StatusBadRequest, "parametro start non valido (formato YYYY-MM-DD)")
			return
		}
		weekStart = db.MondayOf(parsed)
	} else {
		weekStart = db.MondayOf(time.Now())
	}

	plan, err := a.WeeklyPlans.GetOrCreateForWeek(ctx, user.ID, weekStart)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "errore caricamento piano")
		return
	}
	full, err := a.WeeklyPlans.LoadFull(ctx, plan)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "errore caricamento piano")
		return
	}

	days := make([]apiDayMacros, 7)
	for i, meals := range full.Days {
		m := macros.ForDay(meals)
		date := weekStart.AddDate(0, 0, i)
		days[i] = apiDayMacros{
			Date:    date.Format("2006-01-02"),
			Kcal:    round2(m.Kcal),
			Protein: round2(m.Protein),
			Carbs:   round2(m.Carbs),
			Fat:     round2(m.Fat),
		}
	}

	writeJSON(w, http.StatusOK, apiWeekResponse{
		WeekStart: weekStart.Format("2006-01-02"),
		Days:      days,
	})
}

func round2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}

// ---- handler GET /api/v1/ingredients ----------------------------------------
// Query params: ?q=query&limit=20

func (a *App) handleAPIIngredients(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	items, err := a.Ingredients.SearchActive(r.Context(), q, limit)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "errore ricerca ingredienti")
		return
	}

	out := make([]apiIngredientResponse, 0, len(items))
	for _, ing := range items {
		brand := ""
		if ing.Brand != nil {
			brand = *ing.Brand
		}
		out = append(out, apiIngredientResponse{
			ID:            ing.ID,
			Name:          ing.Name,
			Brand:         brand,
			Unit:          string(ing.Unit),
			KcalPer100:    ing.KcalPer100,
			ProteinPer100: ing.ProteinPer100,
			CarbsPer100:   ing.CarbsPer100,
			FatPer100:     ing.FatPer100,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// ---- handler POST /api/v1/water ---------------------------------------------
// Body JSON: {"date":"YYYY-MM-DD","amount_ml":250}

func (a *App) handleAPILogWater(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	var body struct {
		Date     string `json:"date"`
		AmountML int    `json:"amount_ml"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "JSON non valido")
		return
	}
	date, err := time.Parse("2006-01-02", body.Date)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "campo date non valido (formato YYYY-MM-DD)")
		return
	}
	if body.AmountML <= 0 {
		writeJSONErr(w, http.StatusBadRequest, "amount_ml deve essere positivo")
		return
	}
	if err := a.WaterLogs.Log(r.Context(), user.ID, date, body.AmountML); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "errore salvataggio idratazione")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"ok": "true"})
}

// ---- handler POST /api/v1/weight --------------------------------------------
// Body JSON: {"date":"YYYY-MM-DD","weight_kg":70.5}

func (a *App) handleAPILogWeight(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	var body struct {
		Date     string  `json:"date"`
		WeightKg float64 `json:"weight_kg"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "JSON non valido")
		return
	}
	date, err := time.Parse("2006-01-02", body.Date)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "campo date non valido (formato YYYY-MM-DD)")
		return
	}
	if body.WeightKg <= 0 || body.WeightKg > 500 {
		writeJSONErr(w, http.StatusBadRequest, "weight_kg non valido")
		return
	}
	id, err := a.WeightLogs.LogWeight(r.Context(), user.ID, date, body.WeightKg, nil)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "errore salvataggio peso")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

// ---- handler POST /api/v1/workout -------------------------------------------
// Body JSON: {"date":"YYYY-MM-DD","activity_id":1,"duration_minutes":30}

func (a *App) handleAPILogWorkout(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	var body struct {
		Date            string `json:"date"`
		ActivityID      int64  `json:"activity_id"`
		DurationMinutes int    `json:"duration_minutes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "JSON non valido")
		return
	}
	date, err := time.Parse("2006-01-02", body.Date)
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "campo date non valido (formato YYYY-MM-DD)")
		return
	}
	if body.ActivityID <= 0 {
		writeJSONErr(w, http.StatusBadRequest, "activity_id obbligatorio")
		return
	}
	if body.DurationMinutes <= 0 {
		writeJSONErr(w, http.StatusBadRequest, "duration_minutes deve essere positivo")
		return
	}
	id, err := a.WorkoutLogs.CreateWorkoutLog(r.Context(), user.ID, body.ActivityID, date, body.DurationMinutes, nil)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "errore salvataggio allenamento")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

// ---- handler GET /api/v1/activities -----------------------------------------

func (a *App) handleAPIActivities(w http.ResponseWriter, r *http.Request) {
	activities, err := a.WorkoutLogs.ListActivities(r.Context())
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "errore caricamento attività")
		return
	}
	writeJSON(w, http.StatusOK, activities)
}
