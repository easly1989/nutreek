// Command i18ncheck checks the translation catalogs and lists, per community
// language, the keys still to translate. It exits with 1 on errors (same
// rules as the TestCatalogs test) and 0 otherwise, warnings included.
//
//	go run ./cmd/i18ncheck
//
// Under GitHub Actions (GITHUB_ACTIONS=true) it also prints ::warning::
// and ::error:: workflow commands, which show as annotations on the run.
package main

import (
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/amon2126/nutreek/internal/i18n"
)

func main() {
	os.Exit(run(os.Stdout, i18n.CheckEmbedded(), os.Getenv("GITHUB_ACTIONS") == "true"))
}

// run prints the report and returns the exit code.
func run(w io.Writer, r i18n.Report, github bool) int {
	for _, e := range r.Errors {
		fmt.Fprintf(w, "error: %s\n", e)
		if github {
			fmt.Fprintf(w, "::error title=Translation catalogs::%s\n", escape(e))
		}
	}
	for _, code := range slices.Sorted(maps.Keys(r.Missing)) {
		keys := r.Missing[code]
		if len(keys) == 0 {
			continue
		}
		fmt.Fprintf(w, "warning: %s: %d keys to translate (English is shown until then)\n", code, len(keys))
		byArea := map[string][]string{}
		for _, k := range keys {
			area, _, _ := strings.Cut(k, ".")
			byArea[area] = append(byArea[area], k)
		}
		var areas []string
		for _, area := range slices.Sorted(maps.Keys(byArea)) {
			fmt.Fprintf(w, "  locales/%s/%s.json\n", code, area)
			for _, k := range byArea[area] {
				fmt.Fprintf(w, "    %s\n", k)
			}
			areas = append(areas, fmt.Sprintf("%s (%d)", area, len(byArea[area])))
		}
		// One annotation per language: GitHub shows only a few per step.
		if github {
			fmt.Fprintf(w, "::warning title=Translate %s (%d keys)::These keys fall back to English: %s. Run go run ./cmd/i18ncheck to list them.\n",
				code, len(keys), escape(strings.Join(areas, ", ")))
		}
	}
	if len(r.Errors) > 0 {
		fmt.Fprintf(w, "%d errors\n", len(r.Errors))
		return 1
	}
	if len(r.Missing) == 0 {
		fmt.Fprintln(w, "ok: every language is complete")
	} else {
		fmt.Fprintln(w, "ok: no errors (the warnings above do not fail the build)")
	}
	return 0
}

// escape makes s safe inside a workflow command.
func escape(s string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A").Replace(s)
}
