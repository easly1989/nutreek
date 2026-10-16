package web

import (
	"regexp"
	"testing"
)

func TestStaticVersion(t *testing.T) {
	v := StaticVersion()
	if !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(v) {
		t.Fatalf("StaticVersion = %q, voglio 12 cifre esadecimali", v)
	}
	if again := StaticVersion(); again != v {
		t.Errorf("StaticVersion non stabile: %q poi %q", v, again)
	}
}

func TestAccentColor(t *testing.T) {
	cases := map[string]string{
		"bosco":    "#2F7A55",
		"mare":     "#1D6FA5",
		"tramonto": "#B4511A",
		"grafite":  "#3A4150",
		"":         "#2F7A55", // sconosciuto: il colore di default
		"rosa":     "#2F7A55",
	}
	for in, want := range cases {
		if got := AccentColor(in); got != want {
			t.Errorf("AccentColor(%q) = %q, voglio %q", in, got, want)
		}
	}
}
