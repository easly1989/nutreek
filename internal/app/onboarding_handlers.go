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

// onboardingData is the view model of the onboarding wizard page.
type onboardingData struct {
	authPageData
	Step      int
	Restart   bool
	Steps     [5]int // [1,2,3,4,5] — used in the template for the stepper
	Form      profileForm
	Templates []*domain.MealTemplate
	TDEE      float64 // energy needs from the step 1 data, 0 when it cannot be computed
	Weight    float64 // what step 2 doses protein on (g/kg), 0 when unknown
}

// handleOnboardingPage GET /onboarding — shows the wizard's current step.
func (a *App) handleOnboardingPage(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	restart := r.URL.Query().Get("restart") == "true"

	// User already onboarded with no restart: redirect to the dashboard.
	if user.OnboardingCompletedAt != nil && !restart {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	step := 1
	if s := r.URL.Query().Get("step"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 1 && n <= 5 {
			step = n
		}
	}

	data := onboardingData{
		authPageData: a.baseData(r),
		Step:         step,
		Restart:      restart,
		Steps:        [5]int{1, 2, 3, 4, 5},
		Form:         profileFormFromUser(user),
	}
	data.Bare = true
	data.TDEE = onboardingTDEE(user)
	data.Weight = derefFloat(user.WeightKg)
	if user.WeightKg != nil && user.HeightCm != nil && user.Sex != nil {
		data.Weight = profile.ProteinWeight(*user.WeightKg, *user.HeightCm, *user.Sex)
	}

	// Step 4 needs the available templates.
	if step == 4 {
		tpls, _ := a.MealTemplates.ListByUser(r.Context(), user.ID)
		data.Templates = tpls
	}

	if err := a.Render.Render(w, r, http.StatusOK, "onboarding", data); err != nil {
		a.serverError(w, r, err)
	}
}

// handleOnboardingStep POST /onboarding — handles the submit of every step.
func (a *App) handleOnboardingStep(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	ctx := r.Context()
	_ = r.ParseForm()

	step, _ := strconv.Atoi(r.FormValue("step"))
	restart := r.FormValue("restart") == "true"

	restartSuffix := ""
	if restart {
		restartSuffix = "&restart=true"
	}

	switch step {
	case 1:
		// Save the physical data — permissive: when parsing fails, skip silently.
		if inp := parseOnboardingPhysical(r, profileInput(user)); inp != nil {
			_ = a.Users.UpdateProfile(ctx, user.ID, *inp)
		}
		http.Redirect(w, r, "/onboarding?step=2"+restartSuffix, http.StatusSeeOther)

	case 2:
		// Save the macro goals — permissive.
		if inp := parseOnboardingMacros(r, profileInput(user)); inp != nil {
			_ = a.Users.UpdateProfile(ctx, user.ID, *inp)
		}
		http.Redirect(w, r, "/onboarding?step=3"+restartSuffix, http.StatusSeeOther)

	case 3:
		// Create a template when a name is given (optional step).
		name := strings.TrimSpace(r.FormValue("template_name"))
		if name != "" && withinLen(name, MaxNameLen) {
			_, _ = a.MealTemplates.Create(ctx, user.ID, name)
		}
		http.Redirect(w, r, "/onboarding?step=4"+restartSuffix, http.StatusSeeOther)

	case 4:
		// Apply the template to the current week (optional).
		tplID, _ := strconv.ParseInt(r.FormValue("template_id"), 10, 64)
		if tplID > 0 {
			weekStart := db.MondayOf(time.Now())
			plan, err := a.WeeklyPlans.GetOrCreateForWeek(ctx, user.ID, weekStart)
			if err == nil {
				hasMeals, _ := a.WeeklyPlans.HasMeals(ctx, plan.ID)
				if !hasMeals {
					_ = a.WeeklyPlans.ApplyTemplate(ctx, plan.ID, user.ID, tplID)
				}
			}
		}
		http.Redirect(w, r, "/onboarding?step=5"+restartSuffix, http.StatusSeeOther)

	case 5:
		_ = a.Users.CompleteOnboarding(ctx, user.ID)
		a.Sessions.Put(ctx, "flash", "onboarding.flash.done")
		http.Redirect(w, r, "/", http.StatusSeeOther)

	default:
		http.Redirect(w, r, "/onboarding?step=1"+restartSuffix, http.StatusSeeOther)
	}
}

// parseOnboardingPhysical puts the physical data from the step 1 form on base,
// the current profile. Returns nil when every field is empty (no update needed).
func parseOnboardingPhysical(r *http.Request, base db.UserProfileInput) *db.UserProfileInput {
	weightKg, _ := parseOptionalPositiveFloat(strings.TrimSpace(r.FormValue("weight_kg")))
	heightCm, _ := parseOptionalPositiveFloat(strings.TrimSpace(r.FormValue("height_cm")))
	birthYear, _ := parseOptionalInt(strings.TrimSpace(r.FormValue("birth_year")))
	sex := strings.TrimSpace(r.FormValue("sex"))
	activityLevel := strings.TrimSpace(r.FormValue("activity_level"))

	if weightKg == nil && heightCm == nil && birthYear == nil && sex == "" && activityLevel == "" {
		return nil
	}

	var sexPtr *string
	if sex == "M" || sex == "F" {
		sexPtr = &sex
	}
	var actPtr *string
	if validActivityLevels[activityLevel] {
		actPtr = &activityLevel
	}

	var bmiPtr, bmrPtr *float64
	if profile.CanComputeIndices(weightKg, heightCm, birthYear, sexPtr) {
		bmi := profile.BMI(*weightKg, *heightCm)
		bmr := profile.BMR(*weightKg, *heightCm, profile.AgeFromBirthYear(*birthYear), *sexPtr)
		bmiPtr = &bmi
		bmrPtr = &bmr
	}

	base.WeightKg, base.HeightCm, base.BirthYear, base.Sex = weightKg, heightCm, birthYear, sexPtr
	base.ActivityLevel, base.BMI, base.BMR = actPtr, bmiPtr, bmrPtr
	return &base
}

// parseOnboardingMacros puts the macro targets from the step 2 form on base,
// the current profile.
func parseOnboardingMacros(r *http.Request, base db.UserProfileInput) *db.UserProfileInput {
	targetKcal, _ := parseOptionalNonNegFloat(strings.TrimSpace(r.FormValue("target_kcal")))
	targetProtein, _ := parseOptionalNonNegFloat(strings.TrimSpace(r.FormValue("target_protein")))
	targetCarbs, _ := parseOptionalNonNegFloat(strings.TrimSpace(r.FormValue("target_carbs")))
	targetFat, _ := parseOptionalNonNegFloat(strings.TrimSpace(r.FormValue("target_fat")))

	if targetKcal == nil && targetProtein == nil && targetCarbs == nil && targetFat == nil {
		return nil
	}

	base.TargetKcal, base.TargetProtein, base.TargetCarbs, base.TargetFat = targetKcal, targetProtein, targetCarbs, targetFat
	return &base
}

// onboardingTDEE estimates the energy needs from the profile's physical data,
// 0 when missing. It recomputes the BMR instead of reading the saved one,
// which may be missing or stale.
func onboardingTDEE(user *domain.User) float64 {
	if !profile.CanComputeIndices(user.WeightKg, user.HeightCm, user.BirthYear, user.Sex) {
		return 0
	}
	bmr := profile.BMR(*user.WeightKg, *user.HeightCm, profile.AgeFromBirthYear(*user.BirthYear), *user.Sex)
	mult := 1.4 // activity not given, as in profile.js
	if user.ActivityLevel != nil {
		mult = profile.ActivityFactor(*user.ActivityLevel)
	}
	return profile.TDEE(bmr, mult)
}

func derefFloat(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}
