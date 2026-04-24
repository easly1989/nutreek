package web

import (
	"embed"
	"io/fs"
)

//go:embed all:templates
var templatesFS embed.FS

//go:embed all:static
var staticFS embed.FS

// Templates ritorna il filesystem virtuale con tutti i template.
func Templates() fs.FS {
	return templatesFS
}

// Static ritorna il filesystem virtuale degli asset statici servibili da
// /static/ (CSS, JS, immagini).
func Static() fs.FS {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		// L'embed e' statico: se fallisce e' un bug build-time.
		panic(err)
	}
	return sub
}
