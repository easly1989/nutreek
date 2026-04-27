package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/amon2126/nutreek/internal/db"
	"github.com/amon2126/nutreek/internal/domain"
)

// dayNames usato per render. Index 0 = lunedi'.
var dayNames = []string{"Lun", "Mar", "Mer", "Gio", "Ven", "Sab", "Dom"}

// dayLabel ritorna l'etichetta human-readable per un day_of_week.
func dayLabel(d int) string {
	if d == domain.DayOfWeekEvery {
		return "Ogni giorno"
	}
	if d >= 0 && d < len(dayNames) {
		return dayNames[d]
	}
	return "?"
}

// ---- view models -----------------------------------------------------------

type templatesListData struct {
	authPageData
	Templates []*domain.MealTemplate
}

type templateNewData struct {
	authPageData
	Name  string
	Error string
}

type templateEditData struct {
	authPageData
	Template     *domain.MealTemplateWithSlots
	NameForm     string
	NameError    string
	SlotForm     slotForm
	SlotError    string
	DayOptions   []dayOption
}

type slotForm struct {
	DayOfWeek string
	Name      string
}

type dayOption struct {
	Value int
	Label string
}

func dayOptions() []dayOption {
	out := []dayOption{{Value: domain.DayOfWeekEvery, Label: "Ogni giorno"}}
	for i, n := range dayNames {
		out = append(out, dayOption{Value: i, Label: n})
	}
	return out
}

// ---- handlers --------------------------------------------------------------

func (a *App) handleTemplatesList(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	list, err := a.MealTemplates.ListByUser(r.Context(), user.ID)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	data := templatesListData{authPageData: a.baseData(r), Templates: list}
	if err := a.Render.Render(w, http.StatusOK, "templates_list", data); err != nil {
		a.serverError(w, r, err)
	}
}

func (a *App) handleTemplateNewForm(w http.ResponseWriter, r *http.Request) {
	data := templateNewData{authPageData: a.baseData(r)}
	if err := a.Render.Render(w, http.StatusOK, "template_new", data); err != nil {
		a.serverError(w, r, err)
	}
}

func (a *App) handleTemplateCreate(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	name := strings.TrimSpace(r.PostFormValue("name"))
	if name == "" {
		data := templateNewData{authPageData: a.baseData(r), Name: name, Error: "Nome obbligatorio."}
		if err := a.Render.Render(w, http.StatusUnprocessableEntity, "template_new", data); err != nil {
			a.serverError(w, r, err)
		}
		return
	}
	id, err := a.MealTemplates.Create(r.Context(), user.ID, name)
	if err != nil {
		a.serverError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "Template creato.")
	http.Redirect(w, r, "/templates/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (a *App) handleTemplateEdit(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		http.NotFound(w, r)
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
	data := templateEditData{
		authPageData: a.baseData(r),
		Template:     tpl,
		NameForm:     nameForm,
		NameError:    nameErr,
		SlotForm:     slotF,
		SlotError:    slotErr,
		DayOptions:   dayOptions(),
	}
	status := http.StatusOK
	if nameErr != "" || slotErr != "" {
		status = http.StatusUnprocessableEntity
	}
	if err := a.Render.Render(w, status, "template_edit", data); err != nil {
		a.serverError(w, r, err)
	}
}

func (a *App) handleTemplateRename(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimSpace(r.PostFormValue("name"))
	if name == "" {
		tpl, err := a.MealTemplates.GetWithSlots(r.Context(), id, user.ID)
		if err != nil {
			a.handleTemplateError(w, r, err)
			return
		}
		a.renderTemplateEdit(w, r, tpl, name, "Nome obbligatorio.",
			slotForm{DayOfWeek: strconv.Itoa(domain.DayOfWeekEvery)}, "")
		return
	}
	if err := a.MealTemplates.Rename(r.Context(), id, user.ID, name); err != nil {
		a.handleTemplateError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "Template rinominato.")
	http.Redirect(w, r, "/templates/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (a *App) handleTemplateAddSlot(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	dayStr := strings.TrimSpace(r.PostFormValue("day_of_week"))
	name := strings.TrimSpace(r.PostFormValue("slot_name"))
	day, parseErr := strconv.Atoi(dayStr)
	form := slotForm{DayOfWeek: dayStr, Name: name}
	if parseErr != nil || day < -1 || day > 6 {
		a.reRenderTemplate(w, r, id, user.ID, form, "Giorno non valido.")
		return
	}
	if name == "" {
		a.reRenderTemplate(w, r, id, user.ID, form, "Nome del pasto obbligatorio.")
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
		http.NotFound(w, r)
		return
	}
	slotID, err := parseID(r, "slotID")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := a.MealTemplates.RemoveSlot(r.Context(), slotID, user.ID); err != nil {
		if errors.Is(err, db.ErrSlotNotFound) {
			http.Error(w, "slot non trovata", http.StatusNotFound)
			return
		}
		a.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/templates/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (a *App) handleTemplateActivate(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := a.MealTemplates.SetActive(r.Context(), id, user.ID); err != nil {
		a.handleTemplateError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "Template attivato.")
	http.Redirect(w, r, "/templates", http.StatusSeeOther)
}

func (a *App) handleTemplateDelete(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	id, err := parseID(r, "id")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := a.MealTemplates.Delete(r.Context(), id, user.ID); err != nil {
		a.handleTemplateError(w, r, err)
		return
	}
	a.Sessions.Put(r.Context(), "flash", "Template eliminato.")
	http.Redirect(w, r, "/templates", http.StatusSeeOther)
}

// ---- helpers ---------------------------------------------------------------

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
		http.NotFound(w, r)
	case errors.Is(err, db.ErrMealTemplateForbidden):
		http.Error(w, "operazione non consentita", http.StatusForbidden)
	default:
		a.serverError(w, r, err)
	}
}
