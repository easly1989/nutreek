package web

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"sync"
)

// accentColors: il colore d'accento di ogni tema nel modo chiaro, lo stesso
// di --acc-l in web/tailwind/input.css. Serve fuori dal CSS: theme-color
// nella pagina e nel manifest dell'app installata (#59).
var accentColors = map[string]string{
	"bosco":    "#2F7A55",
	"mare":     "#1D6FA5",
	"tramonto": "#B4511A",
	"grafite":  "#3A4150",
}

// AccentColor restituisce il colore del tema; quello di default (bosco) per
// un nome sconosciuto.
func AccentColor(name string) string {
	if c, ok := accentColors[name]; ok {
		return c
	}
	return accentColors["bosco"]
}

var (
	staticVersionOnce sync.Once
	staticVersion     string
)

// StaticVersion e' un'impronta del contenuto di /static: cambia appena cambia
// un file. Il service worker la mette nel nome della sua cache, cosi' dopo un
// aggiornamento il browser butta i file vecchi. Calcolata una volta sola.
func StaticVersion() string {
	staticVersionOnce.Do(func() {
		h := sha256.New()
		static := Static()
		err := fs.WalkDir(static, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, err := fs.ReadFile(static, path)
			if err != nil {
				return err
			}
			h.Write([]byte(path))
			h.Write([]byte{0})
			h.Write(data)
			return nil
		})
		if err != nil {
			// L'embed e' statico: se fallisce e' un bug build-time.
			panic(err)
		}
		staticVersion = hex.EncodeToString(h.Sum(nil))[:12]
	})
	return staticVersion
}
