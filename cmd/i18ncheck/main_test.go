package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/amon2126/nutreek/internal/i18n"
)

func TestRun(t *testing.T) {
	warn := i18n.Report{Missing: map[string][]string{"de": {"week.add_meal", "week.cheat", "plan.title"}}}
	fail := i18n.Report{Errors: []string{`it: missing key "week.cheat"`}, Missing: map[string][]string{}}

	tests := []struct {
		name   string
		report i18n.Report
		github bool
		code   int
		want   []string
	}{
		{"warnings pass", warn, false, 0, []string{
			"warning: de: 3 keys to translate", "  locales/de/plan.json\n    plan.title", "  locales/de/week.json\n    week.add_meal\n    week.cheat", "ok: no errors"}},
		{"annotations on GitHub", warn, true, 0, []string{
			"::warning title=Translate de (3 keys)::These keys fall back to English: plan (1), week (2). Run go run ./cmd/i18ncheck"}},
		{"errors fail", fail, false, 1, []string{`error: it: missing key "week.cheat"`, "1 errors"}},
		{"error annotation", fail, true, 1, []string{`::error title=Translation catalogs::it: missing key "week.cheat"`}},
		{"complete", i18n.Report{}, false, 0, []string{"ok: every language is complete"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if got := run(&out, tt.report, tt.github); got != tt.code {
				t.Errorf("exit code = %d, want %d", got, tt.code)
			}
			for _, w := range tt.want {
				if !strings.Contains(out.String(), w) {
					t.Errorf("output lacks %q:\n%s", w, out.String())
				}
			}
			if !tt.github && strings.Contains(out.String(), "::") {
				t.Errorf("workflow commands outside GitHub:\n%s", out.String())
			}
		})
	}
}
