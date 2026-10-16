package app

import "testing"

func TestLocalPath(t *testing.T) {
	const fb = "/piano-nutrizionale?tab=template"
	cases := map[string]string{
		"/templates/3":         "/templates/3",
		"":                     fb,
		"https://evil.example": fb,
		"//evil.example/x":     fb,
		"/\\evil.example":      fb,
		"templates/3":          fb,
		"javascript:alert(1)":  fb,
		// I browser tolgono tab e a capo dagli URL: "/\t/x" diventerebbe "//x".
		"/\t/evil.example":  fb,
		"/\n\\evil.example": fb,
	}
	for in, want := range cases {
		if got := localPath(in, fb); got != want {
			t.Errorf("localPath(%q) = %q, atteso %q", in, got, want)
		}
	}
}
