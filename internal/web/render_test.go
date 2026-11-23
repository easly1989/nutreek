package web

import "testing"

func TestNavSection(t *testing.T) {
	for cur, want := range map[string]string{
		"/":                   "week",
		"/shopping-list":      "shopping",
		"/recipes/12/edit":    "library",
		"/ingredients":        "library",
		"/piano-nutrizionale": "library",
		"/templates/3":        "library",
		"/stats":              "progress",
		"/profile/peso":       "progress",
		"/profile":            "you",
		"/tu":                 "you",
		"/settings/household": "you",
		"/login":              "",
		"/profile-old":        "",
		"/recipes-old":        "",
	} {
		if got := navSection(cur); got != want {
			t.Errorf("navSection(%q) = %q, want %q", cur, got, want)
		}
	}
}

func TestNavActive(t *testing.T) {
	cases := []struct {
		cur, href string
		want      bool
	}{
		{"/", "/", true},
		{"/recipes", "/", false},
		{"/recipes", "/recipes", true},
		{"/recipes/12/edit", "/recipes", true},
		{"/recipes-old", "/recipes", false},
		{"/profile/peso", "/profile/peso", true},
		{"/stats", "/shopping-list", false},
	}
	for _, tc := range cases {
		if got := navActive(tc.cur, tc.href); got != tc.want {
			t.Errorf("navActive(%q, %q) = %v, want %v", tc.cur, tc.href, got, tc.want)
		}
	}
}
