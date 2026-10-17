package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/go-chi/chi/v5"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
)

// ---- view models -----------------------------------------------------------

type templateNewData struct {
	authPageData
	Name  string
	Error string
}

type templateEditData struct {
	authPageData
	Template   *domain.MealTemplateWithSlots
	Templates  []*domain.MealTemplate // every template of the user (right column)
	SlotGroups []slotGroup
	NameForm   string
	NameError  string
	SlotForm   slotForm
	SlotError  string
}

// slotGroup groups the slots by day for the drag-and-drop UI.
// Every group has its own sortable container in template_edit.html.
type slotGroup struct {
	Day   int
	Label string // "Lunedì", ...; empty for every day, which the template translates
	Slots []domain.MealTemplateSlot
}

var dayFullNames = []string{"Lunedì", "Martedì", "Mercoledì", "Giovedì", "Venerdì", "Sabato", "Domenica"}

// localPath returns next when it is a path of this app ("/..." but not
// "//host" nor "/\host"), otherwise fallback: it prevents an open redirect. No
// control characters: browsers strip them, and "/\t/host" becomes "//host".
func localPath(next, fallback string) string {
	if strings.HasPrefix(next, "/") && !strings.HasPrefix(next, "//") && !strings.HasPrefix(next, "/\\") &&
		!strings.ContainsFunc(next, unicode.IsControl) {
		return next
	}
	return fallback
}

// groupSlotsByDay groups the template's slots by day_of_week, keeping the
// current order (slots already arrive ORDER BY day_of_week, sort_order).
// It always returns 8 groups (-1 + 0..6) even when empty, so drag-and-drop
// works with a single element too.
func groupSlotsByDay(slots []domain.MealTemplateSlot) []slotGroup {
	out := make([]slotGroup, 0, 8)
	for _, day := range []int{domain.DayOfWeekEvery, 0, 1, 2, 3, 4, 5, 6} {
		g := slotGroup{Day: day}
		if day >= 0 {
			g.Label = dayFullNames[day]
		}
		for _, s := range slots {
			if s.DayOfWeek == day {
				g.Slots = append(g.Slots, s)
			}
		}
		out = append(out, g)
	}
	return out
}

type slotForm struct {
	DayOfWeek string
	Name      string
}

// ---- handlers --------------------------------------------------------------

// handleTemplatesList: the template list lives in the «Template settimanali»
// tab of the nutrition plan (#122); /templates redirects there.
func (a *App) handleTemplatesList(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/piano-nutrizionale?tab=template", http.StatusSeeOther)
}

func (a *App) handleTemplateNewForm(w http.ResponseWriter, r *http.Request) {
	data := templateNewData{authPageData: a.baseData(r)}
	if err := a.Render.Render(w, r, http.StatusOK, "template_new", data); err != nil {
		a.serverError(w, r, err)
	}
}

func (a *App) handleTemplateCreate(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	name := strings.TrimSpace(r.PostFormValue("name"))
	if msg := a.templateNameError(r, name); msg != "" {
		data := templateNewData{authPageData: a.baseData(r), Name: name, Error: msg}
		if err := a.Render.Render(w, r, http.StatusUnprocessableEntity, "template_new", data); err != nil {
			a.serverError(w, r, err)
		}
		return
	}
	id, err := a.MealTemplates.Create(r.Context(), user.ID, name)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "templates.flash.created")
	http.Redirect(w, r, "/templates/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (a *App) handleTemplateEdit(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	tpl, err := a.MealTemplates.GetWithSlots(r.Context(), id, user.ID)
	if err != nil {
		a.handleTemplateError(w, r, err)
		return
	}
	a.renderTemplateEdit(w, r, tpl, "", "", slotForm{DayOfWeek: strconv.Itoa(domain.DayOfWeekEvery)}, "")
}

func (a *App) renderTemplateEdit(
	w http.ResponseWriter, r *http.Request,
	tpl *domain.MealTemplateWithSlots,
	nameForm, nameErr string,
	slotF slotForm, slotErr string,
) {
	if nameForm == "" {
		nameForm = tpl.Name
	}
	user := userFromContext(r.Context())
	all, err := a.MealTemplates.ListByUser(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	data := templateEditData{
		authPageData: a.baseData(r),
		Template:     tpl,
		Templates:    all,
		SlotGroups:   groupSlotsByDay(tpl.Slots),
		NameForm:     nameForm,
		NameError:    nameErr,
		SlotForm:     slotF,
		SlotError:    slotErr,
	}
	status := http.StatusOK
	if nameErr != "" || slotErr != "" {
		status = http.StatusUnprocessableEntity
	}
	if err := a.Render.Render(w, r, status, "template_edit", data); err != nil {
		a.serverError(w, r, err)
	}
}

func (a *App) handleTemplateRename(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	name := strings.TrimSpace(r.PostFormValue("name"))
	if msg := a.templateNameError(r, name); msg != "" {
		tpl, err := a.MealTemplates.GetWithSlots(r.Context(), id, user.ID)
		if err != nil {
			a.handleTemplateError(w, r, err)
			return
		}
		a.renderTemplateEdit(w, r, tpl, name, msg,
			slotForm{DayOfWeek: strconv.Itoa(domain.DayOfWeekEvery)}, "")
		return
	}
	if err := a.MealTemplates.Rename(r.Context(), id, user.ID, name); err != nil {
		a.handleTemplateError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "templates.flash.renamed")
	http.Redirect(w, r, "/templates/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (a *App) handleTemplateAddSlot(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	dayStr := strings.TrimSpace(r.PostFormValue("day_of_week"))
	name := strings.TrimSpace(r.PostFormValue("slot_name"))
	day, parseErr := strconv.Atoi(dayStr)
	form := slotForm{DayOfWeek: dayStr, Name: name}
	if parseErr != nil || day < -1 || day > 6 {
		a.reRenderTemplate(w, r, id, user.ID, form, a.t(r, "templates.error.day"))
		return
	}
	if name == "" {
		a.reRenderTemplate(w, r, id, user.ID, form, a.t(r, "templates.error.meal_name_required"))
		return
	}
	if !withinLen(name, MaxNameLen) {
		a.reRenderTemplate(w, r, id, user.ID, form, a.t(r, "templates.error.name_too_long", "Max", MaxNameLen))
		return
	}
	if _, err := a.MealTemplates.AddSlot(r.Context(), id, user.ID, day, name); err != nil {
		a.handleTemplateError(w, r, err)
		return
	}
	http.Redirect(w, r, "/templates/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (a *App) handleTemplateRemoveSlot(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	slotID, err := parseID(r, "slotID")
	if err != nil {
		a.notFound(w, r)
		return
	}
	if err := a.MealTemplates.RemoveSlot(r.Context(), slotID, user.ID); err != nil {
		if errors.Is(err, db.ErrSlotNotFound) {
			http.Error(w, a.t(r, "templates.error.slot_not_found"), http.StatusNotFound)
			return
		}
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/templates/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// handleReorderTemplateSlots POST /templates/{id}/days/{day}/slots/reorder
// Body: ids=12,9,15. Persists the order of the template's slots for the given
// day (-1 for "every day" is supported too). Answers with a redirect to the
// template page (the UI swaps automatically via HTMX after Sortable).
func (a *App) handleReorderTemplateSlots(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	dayStr := chi.URLParam(r, "day")
	day, err := strconv.Atoi(dayStr)
	if err != nil || day < domain.DayOfWeekEvery || day > 6 {
		http.Error(w, a.t(r, "templates.error.day_of_week"), http.StatusBadRequest)
		return
	}
	ids, err := parseIDList(r.PostFormValue("ids"))
	if err != nil {
		http.Error(w, a.t(r, "templates.error.ids"), http.StatusBadRequest)
		return
	}
	if err := a.MealTemplates.ReorderSlots(r.Context(), id, user.ID, day, ids); err != nil {
		if errors.Is(err, db.ErrReorderMismatch) {
			http.Error(w, a.t(r, "templates.error.order"), http.StatusConflict)
			return
		}
		a.handleTemplateError(w, r, err)
		return
	}
	// For now template_edit has no dedicated partial: for HX-Request we
	// answer 204 and the client can update the UI on its own (Sortable
	// already did). For classic navigation, redirect to the page.
	if r.Header.Get("HX-Request") == "true" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, "/templates/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (a *App) handleTemplateActivate(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	if err := a.MealTemplates.SetActive(r.Context(), id, user.ID); err != nil {
		a.handleTemplateError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "templates.flash.activated")
	http.Redirect(w, r, localPath(r.PostFormValue("next"), "/piano-nutrizionale?tab=template"), http.StatusSeeOther)
}

func (a *App) handleTemplateDelete(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		a.notFound(w, r)
		return
	}
	if err := a.MealTemplates.Delete(r.Context(), id, user.ID); err != nil {
		a.handleTemplateError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "templates.flash.deleted")
	http.Redirect(w, r, "/piano-nutrizionale?tab=template", http.StatusSeeOther)
}

// ---- helpers ---------------------------------------------------------------

// templateNameError returns the translated error of an invalid template
// name, or "" when the name is valid.
func (a *App) templateNameError(r *http.Request, name string) string {
	switch {
	case name == "":
		return a.t(r, "templates.error.name_required")
	case !withinLen(name, MaxNameLen):
		return a.t(r, "templates.error.name_too_long", "Max", MaxNameLen)
	}
	return ""
}

func (a *App) reRenderTemplate(w http.ResponseWriter, r *http.Request, id, userID int64, slotF slotForm, slotErr string) {
	tpl, err := a.MealTemplates.GetWithSlots(r.Context(), id, userID)
	if err != nil {
		a.handleTemplateError(w, r, err)
		return
	}
	a.renderTemplateEdit(w, r, tpl, "", "", slotF, slotErr)
}

func (a *App) handleTemplateError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, db.ErrMealTemplateNotFound):
		a.notFound(w, r)
	case errors.Is(err, db.ErrMealTemplateForbidden):
		http.Error(w, a.t(r, "templates.error.forbidden"), http.StatusForbidden)
	default:
		a.serverError(w, r, err)
	}
}
