package i18n

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"regexp"
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
		{"de-DE,de;q=0.9", "en"},           // neither: English
		{"de-DE,it;q=0.5", "it"},           // the first supported one wins
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

// complete are the languages that must translate every key; the others may
// leave keys to the English fallback.
var complete = []string{"it", "en"}

var placeholder = regexp.MustCompile(`{{\s*\.(\w+)\s*}}`)

func placeholders(s string) []string {
	var out []string
	for _, m := range placeholder.FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// pluralForms reads a catalog value: a string, or an object of CLDR plural
// forms that has at least "other".
func pluralForms(raw json.RawMessage) (map[string]string, error) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return map[string]string{"other": s}, nil
	}
	var forms map[string]string
	if err := json.Unmarshal(raw, &forms); err != nil {
		return nil, fmt.Errorf("not a string nor an object of plural forms")
	}
	for form := range forms {
		if !slices.Contains([]string{"zero", "one", "two", "few", "many", "other"}, form) {
			return nil, fmt.Errorf("unknown plural form %q", form)
		}
	}
	if _, ok := forms["other"]; !ok {
		return nil, fmt.Errorf(`plural forms without "other"`)
	}
	return forms, nil
}

// TestCatalogs keeps the catalogs consistent: every file a flat object of
// non-empty strings (or plural forms), keys prefixed with their area, the
// same placeholders as English, and the complete languages missing no key.
func TestCatalogs(t *testing.T) {
	catalogs := map[string]map[string]string{} // language → key → text
	for _, l := range Languages {
		catalogs[l.Code] = map[string]string{}
		files, _ := fs.Glob(locales, "locales/"+l.Code+"/*.json")
		if len(files) == 0 {
			t.Errorf("%s: no catalog files", l.Code)
		}
		for _, f := range files {
			buf, _ := fs.ReadFile(locales, f)
			var m map[string]json.RawMessage
			if err := json.Unmarshal(buf, &m); err != nil {
				t.Errorf("%s: %v", f, err)
				continue
			}
			area := strings.TrimSuffix(path.Base(f), ".json")
			for k, raw := range m {
				if !strings.HasPrefix(k, area+".") {
					t.Errorf("%s: key %q must start with %q", f, k, area+".")
				}
				forms, err := pluralForms(raw)
				if err != nil {
					t.Errorf("%s: key %q: %v", f, k, err)
					continue
				}
				for form, v := range forms {
					if strings.TrimSpace(v) == "" {
						t.Errorf("%s: key %q (%s) is empty", f, k, form)
					}
				}
				// Placeholders are compared over all the forms together.
				catalogs[l.Code][k] = strings.Join(slices.Collect(maps.Values(forms)), "\n")
			}
		}
	}

	en := catalogs["en"]
	for code, cat := range catalogs {
		for k, v := range cat {
			ref, ok := en[k]
			if !ok {
				t.Errorf("%s: key %q is not in English", code, k)
				continue
			}
			if got, want := placeholders(v), placeholders(ref); !slices.Equal(got, want) {
				t.Errorf("%s: key %q has placeholders %v, English has %v", code, k, got, want)
			}
		}
	}
	for _, code := range complete {
		for k := range en {
			if _, ok := catalogs[code][k]; !ok {
				t.Errorf("%s: missing key %q", code, k)
			}
		}
	}
}
