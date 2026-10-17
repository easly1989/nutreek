// Package i18n holds the translation catalogs and the list of languages the
// interface speaks (#141).
//
// The catalogs live in locales/<language>/<area>.json, one flat JSON object
// of key → text per area, in the go-i18n v2 format Weblate reads. Keys are
// English and grouped by area ("ingredients.title"); placeholders use the Go
// template syntax ({{.Count}}). A key missing in a language falls back to
// English, and a key missing in English too is shown as it is.
package i18n

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"strings"

	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

// Language is one language the interface can speak.
type Language struct {
	Code string // BCP 47 tag, also the catalog directory
	Name string // native name, as the language picker shows it
	Beta bool   // translation still incomplete
}

// Languages is the single list of supported languages.
var Languages = []Language{
	{Code: "it", Name: "Italiano"},
	{Code: "en", Name: "English"},
}

// Default is the language of a context that carries none, such as a handler
// called outside the router; requests always resolve to one (see Match).
const Default = "it"

// fallback is the language every other one falls back to.
const fallback = "en"

// matchCodes lists the codes in the matcher's order: English first, so that
// it wins when nothing matches.
var matchCodes, matcher = func() ([]string, language.Matcher) {
	codes := []string{fallback}
	for _, l := range Languages {
		if l.Code != fallback {
			codes = append(codes, l.Code)
		}
	}
	tags := make([]language.Tag, len(codes))
	for i, c := range codes {
		tags[i] = language.Make(c)
	}
	return codes, language.NewMatcher(tags)
}()

// Match returns the supported language that best fits an Accept-Language
// header, or English when none does.
func Match(acceptLanguage string) string {
	tags, _, err := language.ParseAcceptLanguage(acceptLanguage)
	if err != nil || len(tags) == 0 {
		return fallback
	}
	if _, i, conf := matcher.Match(tags...); conf != language.No {
		return matchCodes[i]
	}
	return fallback
}

//go:embed locales
var locales embed.FS

var localizers = mustLoad(locales)

func mustLoad(fsys fs.FS) map[string]*goi18n.Localizer {
	bundle := goi18n.NewBundle(language.Make(fallback))
	out := make(map[string]*goi18n.Localizer, len(Languages))
	for _, l := range Languages {
		files, err := fs.Glob(fsys, "locales/"+l.Code+"/*.json")
		if err != nil {
			panic(err)
		}
		for _, f := range files {
			buf, err := fs.ReadFile(fsys, f)
			if err != nil {
				panic(err)
			}
			// go-i18n reads the language from the file name: area.<code>.json.
			name := strings.TrimSuffix(path.Base(f), ".json") + "." + l.Code + ".json"
			if _, err := bundle.ParseMessageFileBytes(buf, name); err != nil {
				panic(fmt.Errorf("i18n: %s: %w", f, err))
			}
		}
		out[l.Code] = goi18n.NewLocalizer(bundle, l.Code)
	}
	return out
}

// T translates the key into lang. args are placeholder name/value pairs:
// T("en", "ingredients.count", "Count", 3) fills {{.Count}}. A "Count"
// argument (int or int64) also picks the plural form.
func T(lang, key string, args ...any) string {
	return translate(localizers, lang, key, args)
}

// TOr translates the key into lang, or returns fallback when no catalog has
// the key: reference data stored in the database shows its own name then.
func TOr(lang, key, fallback string) string {
	if s := T(lang, key); s != key {
		return s
	}
	return fallback
}

func translate(locs map[string]*goi18n.Localizer, lang, key string, args []any) string {
	loc, ok := locs[lang]
	if !ok {
		loc = locs[Default]
	}
	cfg := &goi18n.LocalizeConfig{MessageID: key}
	if len(args) > 1 {
		data := make(map[string]any, len(args)/2)
		for i := 0; i+1 < len(args); i += 2 {
			name, _ := args[i].(string)
			data[name] = args[i+1]
			// go-i18n picks plural forms only from whole numbers: a float
			// Count would make it fail and show the key.
			switch args[i+1].(type) {
			case int, int64:
				if name == "Count" {
					cfg.PluralCount = args[i+1]
				}
			}
		}
		cfg.TemplateData = data
	}
	// On a key missing in lang, go-i18n returns the English text along with
	// the error: only an empty result means English lacks it too.
	msg, _ := loc.Localize(cfg)
	if msg == "" {
		return key
	}
	return msg
}

// Supported reports whether code is in Languages.
func Supported(code string) bool {
	for _, l := range Languages {
		if l.Code == code {
			return true
		}
	}
	return false
}

type ctxKey struct{}

// WithLang returns a context that carries the request's language.
func WithLang(ctx context.Context, code string) context.Context {
	return context.WithValue(ctx, ctxKey{}, code)
}

// FromContext returns the request's language, or Default.
func FromContext(ctx context.Context) string {
	if code, ok := ctx.Value(ctxKey{}).(string); ok && code != "" {
		return code
	}
	return Default
}
