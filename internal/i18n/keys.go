package i18n

import (
	"encoding/json"
	"io/fs"
	"slices"
)

// Keys returns the message keys of lang's catalogs, sorted. Tests use it to
// tell a page that shows a key as text from one that shows a translation.
func Keys(lang string) []string {
	files, err := fs.Glob(locales, "locales/"+lang+"/*.json")
	if err != nil {
		panic(err)
	}
	var keys []string
	for _, f := range files {
		buf, err := fs.ReadFile(locales, f)
		if err != nil {
			panic(err)
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(buf, &m); err != nil {
			panic(err)
		}
		for k := range m {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return keys
}
