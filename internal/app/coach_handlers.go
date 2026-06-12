package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
)

// requireCoach blocca chi non ha ruolo 'coach'. Risponde 404 (non 403) per
// non rivelare l'esistenza degli endpoint coach agli utenti normali.
func (a *App) requireCoach(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := userFromContext(r.Context())
		if u == nil || u.Role != domain.RoleCoach {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---- /coach -----------------------------------------------------------------

type coachDashboardData struct {
	authPageData
	Clients []domain.CoachClientInfo
	Plans   []domain.NutritionistPlan
}

// handleCoachDashboard GET /coach — clienti, libreria piani e assegnazioni.
func (a *App) handleCoachDashboard(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	clients, err := a.Coach.ListClients(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	plans, err := a.NutritionistPlans.ListPlans(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	data := coachDashboardData{
		authPageData: a.baseData(r),
		Clients:      clients,
		Plans:        plans,
	}
	if err := a.Render.Render(w, http.StatusOK, "coach_dashboard", data); err != nil {
		a.serverError(w, r, err)
	}
}

// handleCoachAddClient POST /coach/clients — aggiunge un cliente per email.
func (a *App) handleCoachAddClient(w http.ResponseWriter, r *http.Request) {
	coach := userFromContext(r.Context())
	email := strings.TrimSpace(r.PostFormValue("email"))
	if email == "" {
		a.coachFlash(w, r, "Inserisci l'email del cliente.")
		return
	}
	client, err := a.Users.GetByEmail(r.Context(), email)
	if errors.Is(err, db.ErrUserNotFound) {
		a.coachFlash(w, r, "Nessun utente registrato con questa email.")
		return
	}
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	if client.ID == coach.ID {
		a.coachFlash(w, r, "Non puoi aggiungere te stesso come cliente.")
		return
	}
	if err := a.Coach.AddClient(r.Context(), coach.ID, client.ID); err != nil {
		if errors.Is(err, db.ErrClientHasCoach) {
			a.coachFlash(w, r, "Questo utente ha gia' un coach.")
			return
		}
		a.serverError(w, r, err)
		return
	}
	a.coachFlash(w, r, "Cliente aggiunto.")
}

// handleCoachRemoveClient POST /coach/clients/{id}/remove
func (a *App) handleCoachRemoveClient(w http.ResponseWriter, r *http.Request) {
	coach := userFromContext(r.Context())
	clientID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := a.Coach.RemoveClient(r.Context(), coach.ID, clientID); err != nil {
		if errors.Is(err, db.ErrCoachClientNotFound) {
			http.NotFound(w, r)
			return
		}
		a.serverError(w, r, err)
		return
	}
	a.coachFlash(w, r, "Cliente rimosso.")
}

// handleCoachAssignPlan POST /coach/clients/{id}/assign-plan — assegna (o
// rimuove, con plan_id vuoto) un piano della libreria del coach al cliente.
func (a *App) handleCoachAssignPlan(w http.ResponseWriter, r *http.Request) {
	coach := userFromContext(r.Context())
	clientID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	planIDStr := strings.TrimSpace(r.PostFormValue("plan_id"))
	if planIDStr == "" {
		if err := a.Coach.UnassignPlan(r.Context(), coach.ID, clientID); err != nil {
			if errors.Is(err, db.ErrCoachClientNotFound) {
				http.NotFound(w, r)
				return
			}
			a.serverError(w, r, err)
			return
		}
		a.coachFlash(w, r, "Assegnazione rimossa: il cliente usa il proprio piano.")
		return
	}
	planID, err := strconv.ParseInt(planIDStr, 10, 64)
	if err != nil {
		a.coachFlash(w, r, "Piano non valido.")
		return
	}
	if err := a.Coach.AssignPlan(r.Context(), coach.ID, clientID, planID); err != nil {
		if errors.Is(err, db.ErrCoachClientNotFound) {
			a.coachFlash(w, r, "Piano o cliente non valido.")
			return
		}
		a.serverError(w, r, err)
		return
	}
	a.coachFlash(w, r, "Piano assegnato al cliente.")
}

// ---- libreria piani del coach -------------------------------------------------

// handleCoachCreatePlan POST /coach/plans — crea un piano nella libreria.
func (a *App) handleCoachCreatePlan(w http.ResponseWriter, r *http.Request) {
	coach := userFromContext(r.Context())
	name := strings.TrimSpace(r.PostFormValue("name"))
	if name == "" || !withinLen(name, MaxNameLen) {
		a.coachFlash(w, r, "Il nome del piano e' obbligatorio (max 200 caratteri).")
		return
	}
	if _, err := a.NutritionistPlans.CreatePlan(r.Context(), coach.ID, name); err != nil {
		a.serverError(w, r, err)
		return
	}
	a.coachFlash(w, r, "Piano creato. Aprilo nell'editor per aggiungere i contenuti.")
}

// handleCoachOpenPlan POST /coach/plans/{id}/open — rende attivo il piano e
// apre l'editor /piano-nutrizionale (l'editor lavora sempre sul piano attivo).
func (a *App) handleCoachOpenPlan(w http.ResponseWriter, r *http.Request) {
	coach := userFromContext(r.Context())
	planID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := a.NutritionistPlans.SetActivePlan(r.Context(), coach.ID, planID); err != nil {
		if errors.Is(err, db.ErrNutritionistPlanNotFound) {
			http.NotFound(w, r)
			return
		}
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/piano-nutrizionale", http.StatusSeeOther)
}

// handleCoachRenamePlan POST /coach/plans/{id}/rename
func (a *App) handleCoachRenamePlan(w http.ResponseWriter, r *http.Request) {
	coach := userFromContext(r.Context())
	planID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimSpace(r.PostFormValue("name"))
	if name == "" || !withinLen(name, MaxNameLen) {
		a.coachFlash(w, r, "Il nome del piano e' obbligatorio (max 200 caratteri).")
		return
	}
	if err := a.NutritionistPlans.RenamePlan(r.Context(), planID, coach.ID, name); err != nil {
		if errors.Is(err, db.ErrNutritionistPlanNotFound) {
			http.NotFound(w, r)
			return
		}
		a.serverError(w, r, err)
		return
	}
	a.coachFlash(w, r, "Piano rinominato.")
}

// handleCoachDeletePlan POST /coach/plans/{id}/delete — solo piani non attivi.
func (a *App) handleCoachDeletePlan(w http.ResponseWriter, r *http.Request) {
	coach := userFromContext(r.Context())
	planID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := a.NutritionistPlans.DeletePlan(r.Context(), planID, coach.ID); err != nil {
		if errors.Is(err, db.ErrNutritionistPlanNotFound) {
			a.coachFlash(w, r, "Il piano attivo non si puo' cancellare: attivane prima un altro.")
			return
		}
		a.serverError(w, r, err)
		return
	}
	a.coachFlash(w, r, "Piano cancellato.")
}

// coachFlash imposta il flash e torna alla dashboard coach.
func (a *App) coachFlash(w http.ResponseWriter, r *http.Request, msg string) {
	a.Sessions.Put(r.Context(), "flash", msg)
	http.Redirect(w, r, "/coach", http.StatusSeeOther)
}
