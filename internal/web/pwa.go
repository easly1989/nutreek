package web

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"sync"
)

// accentColors: the accent color of every theme in light mode, the same as
// --acc-l in web/tailwind/input.css. Needed outside the CSS: theme-color in
// the page and in the installed app's manifest (#59).
var accentColors = map[string]string{
	"bosco":    "#2F7A55",
	"mare":     "#1D6FA5",
	"tramonto": "#B4511A",
	"grafite":  "#3A4150",
}

// AccentColor returns the theme's color; the default one (bosco) for an
// unknown name.
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

// StaticVersion is a fingerprint of the content of /static: it changes as
// soon as a file changes. The service worker puts it in its cache name, so
// after an update the browser drops the old files. Computed only once.
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
			// The embed is static: when it fails it is a build-time bug.
			panic(err)
		}
		staticVersion = hex.EncodeToString(h.Sum(nil))[:12]
	})
	return staticVersion
}
