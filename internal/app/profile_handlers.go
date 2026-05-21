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
	HasIndices  bool // true se BMI/BMR sono calcolati e mostrabili
}

type profileForm struct {
	WeightKg      string
	HeightCm      string
	BirthYear     string
	Sex           string
	TargetKcal    string
	TargetProtein string
	TargetCarbs   string
	TargetFat     string
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
	return f
}

func formatOptFloat(v float64) string {
	// Se è un intero esatto, mostra senza decimali.
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}

// ---- handlers --------------------------------------------------------------

// handleProfilePage GET /profile — mostra la scheda personale.
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
	}
	if err := a.Render.Render(w, http.StatusOK, "profile", data); err != nil {
		a.serverError(w, r, err)
	}
}

// handleProfileSave POST /profile — valida e salva i dati del profilo.
func (a *App) handleProfileSave(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	form := profileForm{
		WeightKg:      strings.TrimSpace(r.PostFormValue("weight_kg")),
		HeightCm:      strings.TrimSpace(r.PostFormValue("height_cm")),
		BirthYear:     strings.TrimSpace(r.PostFormValue("birth_year")),
		Sex:           strings.TrimSpace(r.PostFormValue("sex")),
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
		if err := a.Render.Render(w, http.StatusUnprocessableEntity, "profile", data); err != nil {
			a.serverError(w, r, err)
		}
	}

	// Dati fisici — tutti opzionali.
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
		// non impostato
	default:
		renderErr("Sesso non valido.")
		return
	}

	// Obiettivi macro — tutti opzionali, non negativi.
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

	// Ricalcola BMI e BMR se tutti i dati fisici sono presenti.
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
		TargetKcal:    targetKcal,
		TargetProtein: targetProtein,
		TargetCarbs:   targetCarbs,
		TargetFat:     targetFat,
		BMI:           bmiPtr,
		BMR:           bmrPtr,
	}
	if err := a.Users.UpdateProfile(r.Context(), user.ID, inp); err != nil {
		a.serverError(w, r, err)
		return
	}
	// Aggiorna l'utente in sessione per riflettere i nuovi target subito.
	updatedUser, err := a.Users.GetByID(r.Context(), user.ID)
	if err == nil {
		a.Sessions.Put(r.Context(), "user_id", updatedUser.ID)
	}
	a.Sessions.Put(r.Context(), "flash", "Profilo salvato.")
	http.Redirect(w, r, "/profile", http.StatusSeeOther)
}

// ---- helpers di parsing (opzionali) ----------------------------------------

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
