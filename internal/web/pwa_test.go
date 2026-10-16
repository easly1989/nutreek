package web

import (
	"regexp"
	"testing"
)

func TestStaticVersion(t *testing.T) {
	v := StaticVersion()
	if !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(v) {
		t.Fatalf("StaticVersion = %q, want 12 hex digits", v)
	}
	if again := StaticVersion(); again != v {
		t.Errorf("StaticVersion not stable: %q then %q", v, again)
	}
}

// The CSS and JS links carry the version: after a release the URL changes and
// the service worker no longer answers with the old files.
func TestAssetURLVersioned(t *testing.T) {
	asset := funcMap()["asset"].(func(string) string)
	if got, want := asset("css/tailwind.css"), "/static/css/tailwind.css?v="+StaticVersion(); got != want {
		t.Errorf("asset = %q, want %q", got, want)
	}
}

func TestAccentColor(t *testing.T) {
	cases := map[string]string{
		"bosco":    "#2F7A55",
		"mare":     "#1D6FA5",
		"tramonto": "#B4511A",
		"grafite":  "#3A4150",
		"":         "#2F7A55", // unknown: the default color
		"rosa":     "#2F7A55",
	}
	for in, want := range cases {
		if got := AccentColor(in); got != want {
			t.Errorf("AccentColor(%q) = %q, want %q", in, got, want)
		}
	}
}
