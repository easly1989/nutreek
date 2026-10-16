package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadAppearance(t *testing.T) {
	cases := []struct {
		name    string
		cookies map[string]string
		want    Appearance
	}{
		{"no cookie: system + forest", nil, Appearance{Mode: "sistema", Accent: "bosco"}},
		{"cookie nuovi validi", map[string]string{"tema_modo": "scuro", "tema_colore": "mare"}, Appearance{Mode: "scuro", Accent: "mare"}},
		{"invalid values: default", map[string]string{"tema_modo": "viola", "tema_colore": "<script>"}, Appearance{Mode: "sistema", Accent: "bosco"}},
		{"legacy forest", map[string]string{"theme": "forest"}, Appearance{Mode: "chiaro", Accent: "bosco"}},
		{"legacy ocean", map[string]string{"theme": "ocean"}, Appearance{Mode: "chiaro", Accent: "mare"}},
		{"legacy sunset", map[string]string{"theme": "sunset"}, Appearance{Mode: "chiaro", Accent: "tramonto"}},
		{"legacy mono diventa grafite scuro", map[string]string{"theme": "mono"}, Appearance{Mode: "scuro", Accent: "grafite"}},
		{"the new cookies win over the legacy one", map[string]string{"theme": "mono", "tema_modo": "chiaro", "tema_colore": "tramonto"}, Appearance{Mode: "chiaro", Accent: "tramonto"}},
		{"new mode only, color from the legacy cookie", map[string]string{"theme": "ocean", "tema_modo": "scuro"}, Appearance{Mode: "scuro", Accent: "mare"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			for k, v := range tc.cookies {
				r.AddCookie(&http.Cookie{Name: k, Value: v})
			}
			if got := readAppearance(r); got != tc.want {
				t.Errorf("got %+v want %+v", got, tc.want)
			}
		})
	}
}
