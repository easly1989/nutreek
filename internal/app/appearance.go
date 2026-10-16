package app

import "net/http"

// Appearance is the user's appearance choice: mode (light, dark, like the
// system) and accent color. It lives in two cookies read on the server, so
// <html> is born with the right data-mode/data-accent and there is no flash
// of the wrong theme on load. Values match web/tailwind/input.css and
// internal/web/static/js/theme.js.
type Appearance struct {
	Mode   string // "chiaro" | "scuro" | "sistema"
	Accent string // "bosco" | "mare" | "tramonto" | "grafite"
}

const (
	cookieMode   = "tema_modo"
	cookieAccent = "tema_colore"
	// cookieLegacyTheme is the cookie of the old four-theme picker.
	cookieLegacyTheme = "theme"
)

var (
	validModes   = map[string]bool{"chiaro": true, "scuro": true, "sistema": true}
	validAccents = map[string]bool{"bosco": true, "mare": true, "tramonto": true, "grafite": true}
	// legacyThemes converts the old themes: they were full palettes, "mono"
	// was the only dark one.
	legacyThemes = map[string]Appearance{
		"forest": {Mode: "chiaro", Accent: "bosco"},
		"ocean":  {Mode: "chiaro", Accent: "mare"},
		"sunset": {Mode: "chiaro", Accent: "tramonto"},
		"mono":   {Mode: "scuro", Accent: "grafite"},
	}
	defaultAppearance = Appearance{Mode: "sistema", Accent: "bosco"}
)

// readAppearance reads the appearance cookies. Every missing or invalid value
// falls back to the old `theme` cookie, when present, and then to the default.
func readAppearance(r *http.Request) Appearance {
	out := defaultAppearance
	if c, err := r.Cookie(cookieLegacyTheme); err == nil {
		if legacy, ok := legacyThemes[c.Value]; ok {
			out = legacy
		}
	}
	if c, err := r.Cookie(cookieMode); err == nil && validModes[c.Value] {
		out.Mode = c.Value
	}
	if c, err := r.Cookie(cookieAccent); err == nil && validAccents[c.Value] {
		out.Accent = c.Value
	}
	return out
}
