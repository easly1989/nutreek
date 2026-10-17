package i18n

import (
	"context"
	"maps"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func TestTranslateFallback(t *testing.T) {
	locs := mustLoad(fstest.MapFS{
		"locales/it/demo.json": {Data: []byte(`{"demo.both": "Ciao {{.Name}}", "demo.plural": "{{.Count}} cose"}`)},
		"locales/en/demo.json": {Data: []byte(`{"demo.both": "Hello {{.Name}}", "demo.en_only": "Only English",
			"demo.plural": {"one": "{{.Count}} thing", "other": "{{.Count}} things"}}`)},
	})
	tests := []struct {
		name, lang, key string
		args            []any
		want            string
	}{
		{"italian", "it", "demo.both", []any{"Name", "Ada"}, "Ciao Ada"},
		{"english", "en", "demo.both", []any{"Name", "Ada"}, "Hello Ada"},
		{"missing in italian falls back to english", "it", "demo.en_only", nil, "Only English"},
		{"missing everywhere shows the key", "it", "demo.nowhere", nil, "demo.nowhere"},
		{"unknown language uses the default", "xx", "demo.both", []any{"Name", "Ada"}, "Ciao Ada"},
		{"plural one", "en", "demo.plural", []any{"Count", 1}, "1 thing"},
		{"plural other", "en", "demo.plural", []any{"Count", 3}, "3 things"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := translate(locs, tt.lang, tt.key, tt.args); got != tt.want {
				t.Errorf("translate(%q, %q) = %q, want %q", tt.lang, tt.key, got, tt.want)
			}
		})
	}
}

func TestTOr(t *testing.T) {
	if got := TOr("en", "reference.allergen.gluten", "glutine"); got != "gluten" {
		t.Errorf("known key: got %q, want gluten", got)
	}
	if got := TOr("it", "reference.allergen.gluten", "x"); got != "glutine" {
		t.Errorf("known key in Italian: got %q, want glutine", got)
	}
	if got := TOr("en", "reference.allergen.saffron", "zafferano"); got != "zafferano" {
		t.Errorf("missing key: got %q, want the fallback", got)
	}
}

func TestMatch(t *testing.T) {
	cases := []struct{ header, want string }{
		{"", "en"},
		{"it-IT,it;q=0.9,en-US;q=0.8", "it"},
		{"it", "it"},
		{"en-GB,en;q=0.9", "en"},
		{"de-DE,de;q=0.9", "en"},            // neither: English
		{"de-DE,it;q=0.5", "it"},            // the first supported one wins
		{"fr-CH, fr;q=0.9, en;q=0.8", "en"}, // English listed after an unsupported one
		{"not a header", "en"},
	}
	for _, c := range cases {
		if got := Match(c.header); got != c.want {
			t.Errorf("Match(%q) = %q, want %q", c.header, got, c.want)
		}
	}
}

func TestContextLanguage(t *testing.T) {
	if got := FromContext(context.Background()); got != Default {
		t.Errorf("empty context: got %q, want %q", got, Default)
	}
	if got := FromContext(WithLang(context.Background(), "en")); got != "en" {
		t.Errorf("got %q, want en", got)
	}
}

// TestCatalogs keeps the real catalogs consistent (see Check). The keys a
// community language lacks are logged (go test -v), not failures.
func TestCatalogs(t *testing.T) {
	r := CheckEmbedded()
	for _, e := range r.Errors {
		t.Error(e)
	}
	for code, keys := range r.Missing {
		t.Logf("%s: %d keys to translate (English is shown for now)", code, len(keys))
	}
}

// memFS builds an in-memory locales tree from "lang/area" → JSON.
func memFS(files map[string]string) fstest.MapFS {
	m := fstest.MapFS{}
	for name, data := range files {
		m["locales/"+name+".json"] = &fstest.MapFile{Data: []byte(data)}
	}
	return m
}

func TestCheck(t *testing.T) {
	base := map[string]string{
		"en/demo": `{"demo.title": "Title", "demo.hello": "Hello {{.Name}}", "demo.items": {"one": "{{.Count}} item", "other": "{{.Count}} items"}}`,
		"it/demo": `{"demo.title": "Titolo", "demo.hello": "Ciao {{.Name}}", "demo.items": {"one": "{{.Count}} voce", "other": "{{.Count}} voci"}}`,
	}
	with := func(extra map[string]string) fstest.MapFS {
		files := maps.Clone(base)
		maps.Copy(files, extra)
		return memFS(files)
	}
	codes := []string{"it", "en", "de"}
	strict := []string{"it", "en"}

	tests := []struct {
		name        string
		fsys        fstest.MapFS
		wantErrors  []string // substrings, one per expected error
		wantMissing []string // keys missing in "de"
	}{
		{"community language complete", with(map[string]string{
			"de/demo": `{"demo.title": "Titel", "demo.hello": "Hallo {{.Name}}", "demo.items": {"one": "{{.Count}} Eintrag", "other": "{{.Count}} Einträge"}}`,
		}), nil, nil},
		{"community language missing keys only warns", with(map[string]string{
			"de/demo": `{"demo.title": "Titel"}`,
		}), nil, []string{"demo.hello", "demo.items"}},
		{"community language without an area file only warns", with(map[string]string{
			"de/other": `{"other.x": "x"}`,
		}), []string{`key "other.x" is not in English`}, []string{"demo.hello", "demo.items", "demo.title"}},
		{"community language with a key English lacks fails", with(map[string]string{
			"de/demo": `{"demo.title": "Titel", "demo.hello": "Hallo {{.Name}}", "demo.items": "{{.Count}} x", "demo.extra": "Zusatz"}`,
		}), []string{`"demo.extra" is not in English`}, nil},
		{"community language with an empty value fails", with(map[string]string{
			"de/demo": `{"demo.title": " ", "demo.hello": "Hallo {{.Name}}", "demo.items": "{{.Count}} x"}`,
		}), []string{`"demo.title" (other) is empty`}, nil},
		{"community language with other placeholders fails", with(map[string]string{
			"de/demo": `{"demo.title": "Titel", "demo.hello": "Hallo {{.Nom}}", "demo.items": "{{.Count}}"}`,
		}), []string{`"demo.hello" has placeholders [Nom], English has [Name]`}, nil},
		{"community language with a key outside its area fails", with(map[string]string{
			"de/demo": `{"demo.title": "Titel", "demo.hello": "Hallo {{.Name}}", "demo.items": "{{.Count}} x", "week.title": "Woche"}`,
		}), []string{`must start with "demo."`, `"week.title" is not in English`}, nil},
		{"community language with a broken file fails", with(map[string]string{
			"de/demo": `{"demo.title": `,
		}), []string{"unexpected end of JSON input"}, []string{"demo.hello", "demo.items", "demo.title"}},
		{"plural forms must have other", with(map[string]string{
			"de/demo": `{"demo.title": "Titel", "demo.hello": "Hallo {{.Name}}", "demo.items": {"one": "{{.Count}} Eintrag"}}`,
		}), []string{`plural forms without "other"`}, []string{"demo.items"}},
		{"folder missing from the language list fails", with(map[string]string{
			"de/demo": `{"demo.title": "Titel", "demo.hello": "Hallo {{.Name}}", "demo.items": "{{.Count}}"}`,
			"fr/demo": `{"demo.title": "Titre"}`,
		}), []string{"locales/fr is not in Languages"}, nil},
		{"language listed without files fails", memFS(base), []string{"de: no catalog files"}, []string{"demo.hello", "demo.items", "demo.title"}},
		{"Italian missing a key fails", memFS(map[string]string{
			"en/demo": base["en/demo"],
			"it/demo": `{"demo.title": "Titolo"}`,
			"de/demo": `{"demo.title": "Titel"}`,
		}), []string{`it: missing key "demo.hello"`, `it: missing key "demo.items"`}, []string{"demo.hello", "demo.items"}},
		{"a key English lacks fails in Italian", with(map[string]string{
			"it/demo": `{"demo.title": "Titolo", "demo.hello": "Ciao {{.Name}}", "demo.items": "{{.Count}} x", "demo.only_it": "solo"}`,
			"de/demo": `{"demo.title": "Titel", "demo.hello": "Hallo {{.Name}}", "demo.items": "{{.Count}} x"}`,
		}), []string{`it: key "demo.only_it" is not in English`}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Check(tt.fsys, codes, strict)
			if len(r.Errors) != len(tt.wantErrors) {
				t.Errorf("errors = %q, want %d matching %q", r.Errors, len(tt.wantErrors), tt.wantErrors)
			}
			for _, want := range tt.wantErrors {
				if !slices.ContainsFunc(r.Errors, func(e string) bool { return strings.Contains(e, want) }) {
					t.Errorf("no error contains %q in %q", want, r.Errors)
				}
			}
			if got := r.Missing["de"]; !slices.Equal(got, tt.wantMissing) {
				t.Errorf("missing in de = %q, want %q", got, tt.wantMissing)
			}
			if len(r.Missing["it"])+len(r.Missing["en"]) > 0 {
				t.Errorf("a complete language is reported as a warning: %v", r.Missing)
			}
		})
	}
}
