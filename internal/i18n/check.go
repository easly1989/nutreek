package i18n

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"
)

// complete are the languages that must translate every key: Italian and
// English, the ones the project maintains. Any other language is a community
// one: a missing key falls back to English and is only reported (#162).
var complete = []string{"it", "en"}

// Report is the result of checking the catalogs.
type Report struct {
	// Errors fail the check: a malformed file, an empty value, a key English
	// lacks, other placeholders than English, or a key missing in a complete
	// language.
	Errors []string
	// Missing lists, per community language, the keys still to translate.
	// They only warn.
	Missing map[string][]string
}

// CheckEmbedded checks the catalogs built into the binary against Languages.
func CheckEmbedded() Report {
	codes := make([]string, len(Languages))
	for i, l := range Languages {
		codes[i] = l.Code
	}
	return Check(locales, codes, complete)
}

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

// Check reads fsys/locales/<code>/*.json for every code and keeps the
// catalogs consistent: each file a flat object of non-empty strings (or plural
// forms), keys prefixed with their area, the same placeholders as English, and
// every code in strict missing no key. Keys a community language lacks are
// reported in Missing, not as errors.
func Check(fsys fs.FS, codes, strict []string) Report {
	r := Report{Missing: map[string][]string{}}
	errorf := func(format string, args ...any) { r.Errors = append(r.Errors, fmt.Sprintf(format, args...)) }

	// A folder nobody listed would never load: the contributor forgot the
	// line in Languages.
	if dirs, err := fs.ReadDir(fsys, "locales"); err == nil {
		for _, d := range dirs {
			if d.IsDir() && !slices.Contains(codes, d.Name()) {
				errorf("locales/%s is not in Languages (internal/i18n/i18n.go): add a line for it", d.Name())
			}
		}
	}

	catalogs := map[string]map[string]string{} // language → key → text
	for _, code := range codes {
		catalogs[code] = map[string]string{}
		files, _ := fs.Glob(fsys, "locales/"+code+"/*.json")
		if len(files) == 0 {
			errorf("%s: no catalog files in locales/%s", code, code)
		}
		for _, f := range files {
			buf, err := fs.ReadFile(fsys, f)
			if err != nil {
				errorf("%s: %v", f, err)
				continue
			}
			var m map[string]json.RawMessage
			if err := json.Unmarshal(buf, &m); err != nil {
				errorf("%s: %v", f, err)
				continue
			}
			area := strings.TrimSuffix(path.Base(f), ".json")
			for _, k := range slices.Sorted(maps.Keys(m)) {
				if !strings.HasPrefix(k, area+".") {
					errorf("%s: key %q must start with %q", f, k, area+".")
				}
				forms, err := pluralForms(m[k])
				if err != nil {
					errorf("%s: key %q: %v", f, k, err)
					continue
				}
				for _, form := range slices.Sorted(maps.Keys(forms)) {
					if strings.TrimSpace(forms[form]) == "" {
						errorf("%s: key %q (%s) is empty", f, k, form)
					}
				}
				// Placeholders are compared over all the forms together.
				catalogs[code][k] = strings.Join(slices.Collect(maps.Values(forms)), "\n")
			}
		}
	}

	en := catalogs[fallback]
	for _, code := range codes {
		for _, k := range slices.Sorted(maps.Keys(catalogs[code])) {
			ref, ok := en[k]
			if !ok {
				errorf("%s: key %q is not in English", code, k)
				continue
			}
			if got, want := placeholders(catalogs[code][k]), placeholders(ref); !slices.Equal(got, want) {
				errorf("%s: key %q has placeholders %v, English has %v", code, k, got, want)
			}
		}
		for _, k := range slices.Sorted(maps.Keys(en)) {
			if _, ok := catalogs[code][k]; ok {
				continue
			}
			if slices.Contains(strict, code) {
				errorf("%s: missing key %q", code, k)
			} else {
				r.Missing[code] = append(r.Missing[code], k)
			}
		}
	}
	return r
}
