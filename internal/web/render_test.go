package web

import "testing"

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
