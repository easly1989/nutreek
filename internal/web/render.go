package web

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path"
	"strings"
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
	}
}
