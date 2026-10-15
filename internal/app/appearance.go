package app

import "net/http"

// Appearance e' la scelta di aspetto dell'utente: modalita' (chiaro, scuro,
// come il sistema) e colore d'accento. Vive in due cookie letti lato server,
// cosi' <html> nasce gia' con data-mode/data-accent giusti e non c'e' lampo
// di tema sbagliato al caricamento. Valori allineati a web/tailwind/input.css
// e internal/web/static/js/theme.js.
type Appearance struct {
	Mode   string // "chiaro" | "scuro" | "sistema"
	Accent string // "bosco" | "mare" | "tramonto" | "grafite"
}

const (
	cookieMode   = "tema_modo"
	cookieAccent = "tema_colore"
	// cookieLegacyTheme e' il cookie del vecchio selettore a quattro temi.
	cookieLegacyTheme = "theme"
)

var (
	validModes   = map[string]bool{"chiaro": true, "scuro": true, "sistema": true}
	validAccents = map[string]bool{"bosco": true, "mare": true, "tramonto": true, "grafite": true}
	// legacyThemes converte i vecchi temi: erano palette complete, "mono"
	// era l'unico scuro.
	legacyThemes = map[string]Appearance{
		"forest": {Mode: "chiaro", Accent: "bosco"},
		"ocean":  {Mode: "chiaro", Accent: "mare"},
		"sunset": {Mode: "chiaro", Accent: "tramonto"},
		"mono":   {Mode: "scuro", Accent: "grafite"},
	}
	defaultAppearance = Appearance{Mode: "sistema", Accent: "bosco"}
)

// readAppearance legge i cookie di aspetto. Ogni valore assente o non valido
// ricade sul vecchio cookie `theme`, se c'e', e poi sul default.
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
