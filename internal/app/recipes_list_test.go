package app

import (
	"testing"

	"github.com/amon2126/nutreek/internal/domain"
)

func TestBuildRecipesList(t *testing.T) {
	row := func(name string, mine, public bool, usages int) recipeListRow {
		return recipeListRow{Recipe: &domain.Recipe{Name: name, IsPublic: public}, Mine: mine, Usages: usages}
	}
	all := []recipeListRow{
		row("Pasta al pomodoro", true, false, 3),
		row("Pollo al curry", true, true, 0),
		row("Frittata", true, false, 0),
		row("Pasta di Bob", false, true, 0),
	}
	names := func(d recipesListData) []string {
		var out []string
		for _, r := range d.Rows {
			out = append(out, r.Recipe.Name)
		}
		return out
	}

	cases := []struct {
		filter, query string
		wantFilter    string
		want          []string
	}{
		{"", "", "mine", []string{"Pasta al pomodoro", "Pollo al curry", "Frittata"}},
		{"sconosciuto", "", "mine", []string{"Pasta al pomodoro", "Pollo al curry", "Frittata"}},
		{"others", "", "others", []string{"Pasta di Bob"}},
		{"used", "", "used", []string{"Pasta al pomodoro"}},
		{"public", "", "public", []string{"Pollo al curry"}},
		{"mine", "PASTA", "mine", []string{"Pasta al pomodoro"}},
	}
	for _, c := range cases {
		d := buildRecipesList(all, c.filter, c.query)
		if d.Filter != c.wantFilter {
			t.Errorf("filter %q: Filter = %q, want %q", c.filter, d.Filter, c.wantFilter)
		}
		got := names(d)
		if len(got) != len(c.want) {
			t.Errorf("filter %q, search %q: %v, want %v", c.filter, c.query, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("filter %q, search %q: %v, want %v", c.filter, c.query, got, c.want)
				break
			}
		}
	}

	// The chip counts ignore the search.
	d := buildRecipesList(all, "mine", "zzz")
	want := map[string]int{"mine": 3, "others": 1, "used": 1, "public": 1}
	for _, f := range d.Filters {
		if f.Count != want[f.Key] {
			t.Errorf("chip %s = %d, want %d", f.Key, f.Count, want[f.Key])
		}
	}
	if d.MineN != 3 || d.OthersN != 1 {
		t.Errorf("MineN/OthersN = %d/%d, want 3/1", d.MineN, d.OthersN)
	}
}
