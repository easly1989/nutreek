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

	"github.com/amon2126/nutreek/internal/off"
)

// Renderer mantiene i template parsati e offre Render per scriverli sulla
// response. Ogni pagina e' una combinazione di layout base + file pagina.
type Renderer struct {
	pages map[string]*template.Template
}

// NewRenderer carica i template dal filesystem dato (tipicamente l'embed).
// Cerca file in templates/layouts/*.html e templates/pages/*.html.
func NewRenderer(tfs fs.FS) (*Renderer, error) {
	layoutFiles, err := fs.Glob(tfs, "templates/layouts/*.html")
	if err != nil {
		return nil, fmt.Errorf("glob layouts: %w", err)
	}
	if len(layoutFiles) == 0 {
		return nil, fmt.Errorf("nessun layout trovato in templates/layouts")
	}
	pageFiles, err := fs.Glob(tfs, "templates/pages/*.html")
	if err != nil {
		return nil, fmt.Errorf("glob pages: %w", err)
	}

	partialFiles, _ := fs.Glob(tfs, "templates/partials/*.html")

	r := &Renderer{pages: make(map[string]*template.Template, len(pageFiles))}
	for _, pageFile := range pageFiles {
		name := strings.TrimSuffix(path.Base(pageFile), ".html")
		files := append([]string{}, layoutFiles...)
		files = append(files, partialFiles...)
		files = append(files, pageFile)
		tpl, err := template.New(name).Funcs(funcMap()).ParseFS(tfs, files...)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", pageFile, err)
		}
		r.pages[name] = tpl
	}
	return r, nil
}

// Render scrive la pagina con il layout "layout". Buffera prima di scrivere
// sulla ResponseWriter cosi' che un errore di template non produca una
// response mezza scritta (che andrebbe con un 200 inutilizzabile).
func (r *Renderer) Render(w http.ResponseWriter, status int, name string, data any) error {
	tpl, ok := r.pages[name]
	if !ok {
		return fmt.Errorf("template %q non registrato", name)
	}
	var buf bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf, "layout", data); err != nil {
		return fmt.Errorf("execute %s: %w", name, err)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, err := buf.WriteTo(w)
	return err
}

// RenderPartial esegue un template definito (di solito un partial caricato
// da templates/partials/*.html) senza wrappare nel layout. Usato per le
// risposte HTMX che inseriscono frammenti nella pagina esistente. `page` e'
// una qualunque pagina registrata: serve solo a localizzare il template,
// dato che ParseFS condivide i partial fra tutte le pagine.
func (r *Renderer) RenderPartial(w http.ResponseWriter, status int, page, partial string, data any) error {
	tpl, ok := r.pages[page]
	if !ok {
		return fmt.Errorf("template %q non registrato", page)
	}
	var buf bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf, partial, data); err != nil {
		return fmt.Errorf("execute partial %s/%s: %w", page, partial, err)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, err := buf.WriteTo(w)
	return err
}

func funcMap() template.FuncMap {
	return template.FuncMap{
		"add": func(a, b int) int { return a + b },
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
		// row passa a un template di riga la riga e la pagina che la
		// contiene (CSRF, settimana): html/template non permette struct literal.
		"row": func(row, page any) RowCtx { return RowCtx{Row: row, Page: page} },
		// neg restituisce il negativo di un float64. Usato nei template per
		// invertire il segno senza eseguire operazioni aritmetiche inline.
		"neg": func(v float64) float64 { return -v },
		// deref dereferenzia un *int64 (0 se nil). Usato per confrontare
		// id opzionali (es. piano assegnato) dentro i template.
		"deref": func(p *int64) int64 {
			if p == nil {
				return 0
			}
			return *p
		},
		"num": formatNum,
		"dec": formatDec,
		// list costruisce una lista di stringhe da usare in un range.
		"list": func(v ...string) []string { return v },
		"liters": func(ml int) string {
			return formatDec(float64(ml)/1000, 2) + " l"
		},
		"sub": func(a, b float64) float64 { return a - b },
		"ringDash": func(pct float64) string {
			// Anello di progresso: cerchio r=44 (circonferenza 276,46).
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
		// mealCtx / itemCtx passano a meal_card e meal_item_row la vista del
		// pasto (o della voce) insieme al contesto della card che serve ai
		// form: html/template non permette struct literal.
		"mealCtx": func(meal any, weekStart, csrf string) MealCtx {
			return MealCtx{Meal: meal, WeekStart: weekStart, CSRFToken: csrf}
		},
		"itemCtx": func(view any, struck, cheat bool, weekStart, csrf string) ItemCtx {
			return ItemCtx{View: view, Struck: struck, Cheat: cheat, WeekStart: weekStart, CSRFToken: csrf}
		},
		// pickerRow passa a picker_row la riga insieme ai dati della lista
		// (tipo di picker, id di destinazione, CSRF).
		// barcode: il codice normalizzato se q e' un codice a barre valido,
		// altrimenti "" (i picker mostrano allora l'esito dello scanner).
		"barcode": func(q string) string {
			code, _ := off.NormalizeBarcode(q)
			return code
		},
		"pickerRow": func(row, list any) PickerRowCtx {
			return PickerRowCtx{Row: row, List: list}
		},
		// featureIcon sceglie l'icona di una funzionalita' premium dalla chiave.
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
		// nav costruisce una voce di navigazione della shell (sidebar e
		// barra in basso): {{template "nav_link" (nav $.ActiveNav "/recipes" "Ricette" "i-recipes")}}.
		"nav": func(cur, href, label, icon string) NavItem {
			return NavItem{Href: href, Label: label, Icon: icon, Active: navActive(cur, href)}
		},
		// initial restituisce la prima lettera maiuscola di un nome, per
		// l'avatar nella sidebar.
		"initial": func(s string) string {
			for _, r := range strings.TrimSpace(s) {
				return strings.ToUpper(string(r))
			}
			return "?"
		},
	}
}

// MealCtx e' il view-model di meal_card: Meal e' la vista del pasto
// costruita dal pacchetto app.
type MealCtx struct {
	Meal      any
	WeekStart string
	CSRFToken string
}

// ItemCtx e' il view-model di meal_item_row. Struck = voce del piano
// sostituita da uno sgarro (barrata); Cheat = voce dello sgarro.
type ItemCtx struct {
	View          any
	Struck, Cheat bool
	WeekStart     string
	CSRFToken     string
}

// PickerRowCtx e' il view-model di picker_row: Row e' la riga ingrediente,
// List la lista del picker che la contiene.
type PickerRowCtx struct {
	Row, List any
}

// RowCtx e' il view-model generico di una riga ripetuta: Row e' la riga,
// Page i dati della pagina.
type RowCtx struct {
	Row, Page any
}

// NavItem e' il view-model di una voce di navigazione della shell.
type NavItem struct {
	Href, Label, Icon string
	Active            bool
}

var (
	weekdaysIT = [7]string{"Lunedì", "Martedì", "Mercoledì", "Giovedì", "Venerdì", "Sabato", "Domenica"}
	monthsIT   = [12]string{"gennaio", "febbraio", "marzo", "aprile", "maggio", "giugno", "luglio", "agosto", "settembre", "ottobre", "novembre", "dicembre"}
)

// FormatNum e FormatDec espongono la formattazione dei template ai frammenti
// HTML scritti a mano dagli handler.
func FormatNum(v float64) string             { return formatNum(v) }
func FormatDec(v float64, digits int) string { return formatDec(v, digits) }

// formatNum arrotonda all'intero e separa le migliaia col punto, come si
// scrive in italiano: 1636.4 -> "1.636".
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

// formatDec scrive v con al massimo `digits` decimali, virgola decimale e
// senza zeri finali: 1.5 -> "1,5", 2.0 -> "2", 1.25 -> "1,25".
func formatDec(v float64, digits int) string {
	s := strconv.FormatFloat(v, 'f', digits, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return strings.Replace(s, ".", ",", 1)
}

// weekRange descrive la settimana che inizia il lunedi' start:
// "12–18 ottobre", "29 settembre – 5 ottobre", "29 dicembre 2025 – 4 gennaio 2026".
func weekRange(start time.Time) string { return weeksRange(start, 1) }

// weeksRange descrive n settimane consecutive dal lunedi' start alla
// domenica dell'ultima: "12 ottobre – 1 novembre".
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

// navActive dice se il link href corrisponde alla pagina corrente cur:
// "/" solo esatto, le altre sezioni anche sulle sottopagine
// (/recipes/12 accende Ricette).
func navActive(cur, href string) bool {
	if href == "/" {
		return cur == "/"
	}
	return cur == href || strings.HasPrefix(cur, href+"/")
}
