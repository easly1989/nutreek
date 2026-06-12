package web

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/amon2126/nutreek/internal/domain"
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

// ShoppingRowAutoData e' il view-model per il partial shopping_row_auto.
// Esposto come tipo pubblico cosi' il funcMap puo' istanziarlo dentro
// un range del template.
type ShoppingRowAutoData struct {
	Item      *domain.ShoppingListItem
	WeekStart string
	CSRFToken string
}

// ShoppingRowUserData e' il view-model per shopping_row_user.
type ShoppingRowUserData struct {
	Item      domain.ShoppingUserItem
	Duplicate bool
	WeekStart string
	CSRFToken string
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
		// rowAuto costruisce il view-model per shopping_row_auto da
		// dentro un range del template (html/template non permette
		// struct literal). Riceve l'item per valore (Items[i] in range).
		"rowAuto": func(item domain.ShoppingListItem, weekStart, csrf string) ShoppingRowAutoData {
			return ShoppingRowAutoData{Item: &item, WeekStart: weekStart, CSRFToken: csrf}
		},
		// rowUser idem per shopping_row_user (chiamato come
		// {{template "shopping_row_user" (rowUser .Item .Duplicate $ws $csrf)}}).
		"rowUser": func(item domain.ShoppingUserItem, dup bool, weekStart, csrf string) ShoppingRowUserData {
			return ShoppingRowUserData{Item: item, Duplicate: dup, WeekStart: weekStart, CSRFToken: csrf}
		},
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
	}
}
