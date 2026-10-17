package web

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"math"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/amon2126/nutreek/internal/i18n"
	"github.com/amon2126/nutreek/internal/off"
)

// Renderer keeps the parsed templates and offers Render to write them to the
// response. Every page is a combination of the base layout + a page file.
type Renderer struct {
	pages map[string]map[string]*template.Template // language → page → template
}

// NewRenderer loads the templates from the given filesystem (usually the
// embed). It looks for files in templates/layouts/*.html and templates/pages/*.html.
// Every page is parsed once per language, with a t function bound to it, so
// {{t "key"}} works in partials without passing the language around.
func NewRenderer(tfs fs.FS) (*Renderer, error) {
	layoutFiles, err := fs.Glob(tfs, "templates/layouts/*.html")
	if err != nil {
		return nil, fmt.Errorf("glob layouts: %w", err)
	}
	if len(layoutFiles) == 0 {
		return nil, fmt.Errorf("no layout found in templates/layouts")
	}
	pageFiles, err := fs.Glob(tfs, "templates/pages/*.html")
	if err != nil {
		return nil, fmt.Errorf("glob pages: %w", err)
	}

	partialFiles, _ := fs.Glob(tfs, "templates/partials/*.html")

	r := &Renderer{pages: make(map[string]map[string]*template.Template, len(i18n.Languages))}
	for _, l := range i18n.Languages {
		funcs := funcMap(l.Code)
		pages := make(map[string]*template.Template, len(pageFiles))
		for _, pageFile := range pageFiles {
			name := strings.TrimSuffix(path.Base(pageFile), ".html")
			files := append([]string{}, layoutFiles...)
			files = append(files, partialFiles...)
			files = append(files, pageFile)
			tpl, err := template.New(name).Funcs(funcs).ParseFS(tfs, files...)
			if err != nil {
				return nil, fmt.Errorf("parse %s: %w", pageFile, err)
			}
			pages[name] = tpl
		}
		r.pages[l.Code] = pages
	}
	return r, nil
}

// page returns the template of the page in the request's language.
func (r *Renderer) page(req *http.Request, name string) (*template.Template, error) {
	tpl, ok := r.pages[i18n.FromContext(req.Context())][name]
	if !ok {
		tpl, ok = r.pages[i18n.Default][name]
	}
	if !ok {
		return nil, fmt.Errorf("template %q not registered", name)
	}
	return tpl, nil
}

// Render writes the page with the "layout" layout. It buffers before writing
// to the ResponseWriter so a template error does not produce a half-written
// response (which would go out with a useless 200).
func (r *Renderer) Render(w http.ResponseWriter, req *http.Request, status int, name string, data any) error {
	tpl, err := r.page(req, name)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf, "layout", data); err != nil {
		return fmt.Errorf("execute %s: %w", name, err)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, err = buf.WriteTo(w)
	return err
}

// RenderPartial runs a defined template (usually a partial loaded from
// templates/partials/*.html) without wrapping it in the layout. Used for the
// HTMX responses that insert fragments into the existing page. `page` is any
// registered page: it only serves to locate the template, since ParseFS
// shares the partials among all the pages.
func (r *Renderer) RenderPartial(w http.ResponseWriter, req *http.Request, status int, page, partial string, data any) error {
	tpl, err := r.page(req, page)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf, partial, data); err != nil {
		return fmt.Errorf("execute partial %s/%s: %w", page, partial, err)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, err = buf.WriteTo(w)
	return err
}

func funcMap(lang string) template.FuncMap {
	return template.FuncMap{
		// t translates a catalog key into the page's language; extra
		// arguments are placeholder name/value pairs: {{t "x.count" "Count" 3}}.
		"t":    func(key string, args ...any) string { return i18n.T(lang, key, args...) },
		"lang": func() string { return lang },
		"add":  func(a, b int) int { return a + b },
		"dayLabel": func(d int) string {
			switch {
			case d == -1:
				return "Ogni giorno"
			case d == 0:
				return "Lun"
			case d == 1:
				return "Mar"
			case d == 2:
				return "Mer"
			case d == 3:
				return "Gio"
			case d == 4:
				return "Ven"
			case d == 5:
				return "Sab"
			case d == 6:
				return "Dom"
			}
			return "?"
		},
		// row passes a row template the row and the page that holds it
		// (CSRF, week): html/template does not allow struct literals.
		"row": func(row, page any) RowCtx { return RowCtx{Row: row, Page: page} },
		// neg returns the negative of a float64. Used in templates to flip the
		// sign without inline arithmetic.
		"neg": func(v float64) float64 { return -v },
		// deref dereferences an *int64 (0 when nil). Used to compare optional
		// ids (e.g. the assigned plan) inside templates.
		"deref": func(p *int64) int64 {
			if p == nil {
				return 0
			}
			return *p
		},
		"num": formatNum,
		"dec": formatDec,
		// list builds a list of strings to use in a range.
		"list": func(v ...string) []string { return v },
		"liters": func(ml int) string {
			return formatDec(float64(ml)/1000, 2) + " l"
		},
		"sub": func(a, b float64) float64 { return a - b },
		"ringDash": func(pct float64) string {
			// Progress ring: a circle with r=44 (circumference 276.46).
			const c = 2 * math.Pi * 44
			pct = math.Max(0, math.Min(pct, 100))
			return fmt.Sprintf("%.1f %.1f", pct/100*c, c)
		},
		"weekRange":  weekRange,
		"weeksRange": weeksRange,
		"dayTitle": func(t time.Time) string {
			return weekdaysIT[(int(t.Weekday())+6)%7] + " " + strconv.Itoa(t.Day())
		},
		"dayShort": func(t time.Time) string {
			return weekdaysIT[(int(t.Weekday())+6)%7][:3] + " " + strconv.Itoa(t.Day())
		},
		"monthName": func(t time.Time) string { return monthsIT[t.Month()-1] },
		"dayLetter": func(t time.Time) string {
			return weekdaysIT[(int(t.Weekday())+6)%7][:1]
		},
		// mealCtx / itemCtx pass meal_card and meal_item_row the view of the
		// meal (or of the item) together with the card's context the forms
		// need: html/template does not allow struct literals.
		"mealCtx": func(meal any, weekStart, csrf string) MealCtx {
			return MealCtx{Meal: meal, WeekStart: weekStart, CSRFToken: csrf}
		},
		"itemCtx": func(view any, struck, cheat bool, weekStart, csrf string) ItemCtx {
			return ItemCtx{View: view, Struck: struck, Cheat: cheat, WeekStart: weekStart, CSRFToken: csrf}
		},
		// barcode: the normalized code when q is a valid barcode, otherwise
		// "". With a code the pickers show the scanner's outcome.
		"barcode": func(q string) string {
			code, _ := off.NormalizeBarcode(q)
			return code
		},
		// accentColor: the theme's color for theme-color (#59).
		"accentColor": AccentColor,
		// asset: the URL of a file in /static with the static assets' version.
		// The service worker serves them from the cache: a new URL on every
		// release avoids old CSS and JS under new HTML.
		"asset": func(path string) string {
			return "/static/" + path + "?v=" + StaticVersion()
		},
		// pickerRow passes picker_row the row together with the list's data
		// (picker kind, target id, CSRF).
		"pickerRow": func(row, list any) PickerRowCtx {
			return PickerRowCtx{Row: row, List: list}
		},
		// featureIcon picks the icon of a premium feature from its key.
		"featureIcon": func(key string) string {
			switch key {
			case "auto_plan":
				return "i-week"
			case "tdee_adaptive":
				return "i-stats"
			case "coach_mode":
				return "i-coach"
			case "api_access":
				return "i-key"
			case "micronutrients":
				return "i-ingredients"
			}
			return "i-star"
		},
		"navActive": navActive,
		// nav builds a navigation entry of the shell (sidebar and bottom
		// bar): {{template "nav_link" (nav $.ActiveNav "/recipes" "Ricette" "i-recipes")}}.
		"nav": func(cur, href, label, icon string) NavItem {
			return NavItem{Href: href, Label: label, Icon: icon, Active: navActive(cur, href)}
		},
		// initial returns the first letter of a name, uppercased, for the
		// avatar in the sidebar.
		"initial": func(s string) string {
			for _, r := range strings.TrimSpace(s) {
				return strings.ToUpper(string(r))
			}
			return "?"
		},
	}
}

// MealCtx is the view model of meal_card: Meal is the meal's view built by
// the app package.
type MealCtx struct {
	Meal      any
	WeekStart string
	CSRFToken string
}

// ItemCtx is the view model of meal_item_row. Struck = a plan item replaced
// by a cheat (struck through); Cheat = an item of the cheat.
type ItemCtx struct {
	View          any
	Struck, Cheat bool
	WeekStart     string
	CSRFToken     string
}

// PickerRowCtx is the view model of picker_row: Row is the ingredient row,
// List the picker list that holds it.
type PickerRowCtx struct {
	Row, List any
}

// RowCtx is the generic view model of a repeated row: Row is the row, Page
// the page's data.
type RowCtx struct {
	Row, Page any
}

// NavItem is the view model of a navigation entry of the shell.
type NavItem struct {
	Href, Label, Icon string
	Active            bool
}

var (
	weekdaysIT = [7]string{"Lunedì", "Martedì", "Mercoledì", "Giovedì", "Venerdì", "Sabato", "Domenica"}
	monthsIT   = [12]string{"gennaio", "febbraio", "marzo", "aprile", "maggio", "giugno", "luglio", "agosto", "settembre", "ottobre", "novembre", "dicembre"}
)

// FormatNum and FormatDec expose the templates' formatting to the HTML
// fragments the handlers write by hand.
func FormatNum(v float64) string             { return formatNum(v) }
func FormatDec(v float64, digits int) string { return formatDec(v, digits) }

// formatNum rounds to the integer and separates thousands with a dot, as
// Italian writes them: 1636.4 -> "1.636".
func formatNum(v float64) string {
	n := int64(math.Round(v))
	sign := ""
	if n < 0 {
		sign, n = "-", -n
	}
	s := strconv.FormatInt(n, 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "." + s[i:]
	}
	return sign + s
}

// formatDec writes v with at most `digits` decimals, a decimal comma and no
// trailing zeros: 1.5 -> "1,5", 2.0 -> "2", 1.25 -> "1,25".
func formatDec(v float64, digits int) string {
	s := strconv.FormatFloat(v, 'f', digits, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return strings.Replace(s, ".", ",", 1)
}

// weekRange describes the week starting on Monday start:
// "12–18 ottobre", "29 settembre – 5 ottobre", "29 dicembre 2025 – 4 gennaio 2026".
func weekRange(start time.Time) string { return weeksRange(start, 1) }

// weeksRange describes n consecutive weeks from Monday start to the Sunday
// of the last one: "12 ottobre – 1 novembre".
func weeksRange(start time.Time, n int) string {
	end := start.AddDate(0, 0, 7*max(n, 1)-1)
	switch {
	case start.Year() != end.Year():
		return fmt.Sprintf("%d %s %d – %d %s %d", start.Day(), monthsIT[start.Month()-1], start.Year(), end.Day(), monthsIT[end.Month()-1], end.Year())
	case start.Month() != end.Month():
		return fmt.Sprintf("%d %s – %d %s", start.Day(), monthsIT[start.Month()-1], end.Day(), monthsIT[end.Month()-1])
	default:
		return fmt.Sprintf("%d–%d %s", start.Day(), end.Day(), monthsIT[end.Month()-1])
	}
}

// navActive says whether the link href matches the current page cur: "/"
// only exactly, the other sections on their subpages too (/recipes/12 lights
// up Ricette).
func navActive(cur, href string) bool {
	if href == "/" {
		return cur == "/"
	}
	return cur == href || strings.HasPrefix(cur, href+"/")
}
