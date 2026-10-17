package app

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
	"github.com/amon2126/nutreek/internal/profile"
)

// ---- view models -----------------------------------------------------------

type profileData struct {
	authPageData
	Form        profileForm
	Error       string
	BMIValue    float64
	BMICategory string
	BMRValue    float64
	TDEEValue   float64
	HasIndices  bool // true when BMI/BMR are computed and can be shown
	HasTDEE     bool // true when activity_level is set too
}

type profileForm struct {
	WeightKg      string
	HeightCm      string
	BirthYear     string
	Sex           string
	ActivityLevel string
	TargetKcal    string
	TargetProtein string
	TargetCarbs   string
	TargetFat     string
	WaterTargetML string
}

var validActivityLevels = map[string]bool{
	"sedentary": true,
	"light":     true,
	"moderate":  true,
	"active":    true,
	"athlete":   true,
}

func profileFormFromUser(u *domain.User) profileForm {
	f := profileForm{}
	if u.WeightKg != nil {
		f.WeightKg = formatOptFloat(*u.WeightKg)
	}
	if u.HeightCm != nil {
		f.HeightCm = formatOptFloat(*u.HeightCm)
	}
	if u.BirthYear != nil {
		f.BirthYear = strconv.Itoa(*u.BirthYear)
	}
	if u.Sex != nil {
		f.Sex = *u.Sex
	}
	if u.ActivityLevel != nil {
		f.ActivityLevel = *u.ActivityLevel
	}
	if u.TargetKcal != nil {
		f.TargetKcal = formatOptFloat(*u.TargetKcal)
	}
	if u.TargetProtein != nil {
		f.TargetProtein = formatOptFloat(*u.TargetProtein)
	}
	if u.TargetCarbs != nil {
		f.TargetCarbs = formatOptFloat(*u.TargetCarbs)
	}
	if u.TargetFat != nil {
		f.TargetFat = formatOptFloat(*u.TargetFat)
	}
	if u.WaterTargetML > 0 {
		f.WaterTargetML = strconv.Itoa(u.WaterTargetML)
	}
	return f
}

func formatOptFloat(v float64) string {
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}

// ---- handlers --------------------------------------------------------------

// handleProfilePage GET /profile — shows the personal profile.
func (a *App) handleProfilePage(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	data := profileData{
		authPageData: a.baseData(r),
		Form:         profileFormFromUser(user),
	}
	if user.BMI != nil && user.BMR != nil {
		data.BMIValue = *user.BMI
		data.BMICategory = profile.BMICategory(*user.BMI)
		data.BMRValue = *user.BMR
		data.HasIndices = true
		if user.ActivityLevel != nil {
			data.TDEEValue = profile.TDEE(*user.BMR, profile.ActivityFactor(*user.ActivityLevel))
			data.HasTDEE = true
		}
	}
	if err := a.Render.Render(w, r, http.StatusOK, "profile", data); err != nil {
		a.serverError(w, r, err)
	}
}

// handleProfileSave POST /profile — validates and saves the profile data.
func (a *App) handleProfileSave(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	form := profileForm{
		WeightKg:      strings.TrimSpace(r.PostFormValue("weight_kg")),
		HeightCm:      strings.TrimSpace(r.PostFormValue("height_cm")),
		BirthYear:     strings.TrimSpace(r.PostFormValue("birth_year")),
		Sex:           strings.TrimSpace(r.PostFormValue("sex")),
		ActivityLevel: strings.TrimSpace(r.PostFormValue("activity_level")),
		WaterTargetML: strings.TrimSpace(r.PostFormValue("water_target_ml")),
		TargetKcal:    strings.TrimSpace(r.PostFormValue("target_kcal")),
		TargetProtein: strings.TrimSpace(r.PostFormValue("target_protein")),
		TargetCarbs:   strings.TrimSpace(r.PostFormValue("target_carbs")),
		TargetFat:     strings.TrimSpace(r.PostFormValue("target_fat")),
	}

	renderErr := func(msg string) {
		data := profileData{
			authPageData: a.baseData(r),
			Form:         form,
			Error:        msg,
		}
		if err := a.Render.Render(w, r, http.StatusUnprocessableEntity, "profile", data); err != nil {
			a.serverError(w, r, err)
		}
	}

	// Physical data — all optional.
	weightKg, err := parseOptionalPositiveFloat(form.WeightKg)
	if err != nil || (weightKg != nil && (*weightKg < 20 || *weightKg > 300)) {
		renderErr("Peso non valido (range plausibile: 20–300 kg).")
		return
	}
	heightCm, err := parseOptionalPositiveFloat(form.HeightCm)
	if err != nil || (heightCm != nil && (*heightCm < 100 || *heightCm > 250)) {
		renderErr("Altezza non valida (range plausibile: 100–250 cm).")
		return
	}
	birthYear, err := parseOptionalInt(form.BirthYear)
	currentYear := time.Now().Year()
	if err != nil || (birthYear != nil && (*birthYear < 1900 || *birthYear > currentYear-5)) {
		renderErr("Anno di nascita non plausibile.")
		return
	}

	var sexPtr *string
	switch form.Sex {
	case "M", "F":
		s := form.Sex
		sexPtr = &s
	case "":
		// not set
	default:
		renderErr("Sesso non valido.")
		return
	}

	var activityPtr *string
	if form.ActivityLevel != "" {
		if !validActivityLevels[form.ActivityLevel] {
			renderErr("Livello di attività non valido.")
			return
		}
		al := form.ActivityLevel
		activityPtr = &al
	}

	// Macro goals — all optional, non-negative.
	targetKcal, err := parseOptionalNonNegFloat(form.TargetKcal)
	if err != nil {
		renderErr("Obiettivo Kcal non valido.")
		return
	}
	targetProtein, err := parseOptionalNonNegFloat(form.TargetProtein)
	if err != nil {
		renderErr("Obiettivo Proteine non valido.")
		return
	}
	targetCarbs, err := parseOptionalNonNegFloat(form.TargetCarbs)
	if err != nil {
		renderErr("Obiettivo Carboidrati non valido.")
		return
	}
	targetFat, err := parseOptionalNonNegFloat(form.TargetFat)
	if err != nil {
		renderErr("Obiettivo Grassi non valido.")
		return
	}

	// Hydration target: optional. Empty = keep the current value
	// (0 = the "do not change" sentinel in UpdateProfile).
	waterTargetML := 0
	if form.WaterTargetML != "" {
		v, errW := strconv.Atoi(form.WaterTargetML)
		if errW != nil || v < 250 || v > 10000 {
			renderErr("Target idratazione non valido (250–10000 ml).")
			return
		}
		waterTargetML = v
	}

	// Validate the macro combination (a physically impossible check).
	if targetKcal != nil && *targetKcal > 0 {
		if *targetKcal < 800 {
			renderErr("Obiettivo Kcal troppo basso (minimo consigliato: 800 kcal).")
			return
		}
		if targetProtein != nil && *targetProtein*4 > *targetKcal {
			renderErr("Le proteine da sole superano le calorie totali (valore non possibile).")
			return
		}
		if targetFat != nil && *targetFat*9 > *targetKcal {
			renderErr("I grassi da soli superano le calorie totali (valore non possibile).")
			return
		}
		if targetProtein != nil && targetCarbs != nil && targetFat != nil {
			macroKcal := *targetProtein*4 + *targetCarbs*4 + *targetFat*9
			if macroKcal > *targetKcal*1.10 {
				renderErr("La somma dei macro supera le calorie totali di oltre il 10%.")
				return
			}
		}
	}
	if targetProtein != nil && *targetProtein > 400 {
		renderErr("Obiettivo Proteine non realistico (massimo 400 g).")
		return
	}

	// Recompute BMI and BMR when all the physical data is present.
	var bmiPtr, bmrPtr *float64
	if profile.CanComputeIndices(weightKg, heightCm, birthYear, sexPtr) {
		bmi := profile.BMI(*weightKg, *heightCm)
		bmr := profile.BMR(*weightKg, *heightCm, profile.AgeFromBirthYear(*birthYear), *sexPtr)
		bmiPtr = &bmi
		bmrPtr = &bmr
	}

	inp := db.UserProfileInput{
		WeightKg:      weightKg,
		HeightCm:      heightCm,
		BirthYear:     birthYear,
		Sex:           sexPtr,
		ActivityLevel: activityPtr,
		TargetKcal:    targetKcal,
		TargetProtein: targetProtein,
		TargetCarbs:   targetCarbs,
		TargetFat:     targetFat,
		BMI:           bmiPtr,
		BMR:           bmrPtr,
		WaterTargetML: waterTargetML,
	}
	if err := a.Users.UpdateProfile(r.Context(), user.ID, inp); err != nil {
		a.serverError(w, r, err)
		return
	}
	// Update the user in the session so the new targets show at once.
	updatedUser, err := a.Users.GetByID(r.Context(), user.ID)
	if err == nil {
		a.Sessions.Put(r.Context(), "user_id", updatedUser.ID)
	}
	a.Sessions.Put(r.Context(), "flash", "Profilo salvato.")
	http.Redirect(w, r, "/profile", http.StatusSeeOther)
}

// ---- parsing helpers (optional) -------------------------------------------

func parseOptionalPositiveFloat(s string) (*float64, error) {
	if s == "" {
		return nil, nil
	}
	s = strings.ReplaceAll(s, ",", ".")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, err
	}
	if v <= 0 {
		return nil, errNegative
	}
	return &v, nil
}

func parseOptionalNonNegFloat(s string) (*float64, error) {
	if s == "" {
		return nil, nil
	}
	s = strings.ReplaceAll(s, ",", ".")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, err
	}
	if v < 0 {
		return nil, errNegative
	}
	return &v, nil
}

func parseOptionalInt(s string) (*int, error) {
	if s == "" {
		return nil, nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return nil, err
	}
	return &v, nil
}
