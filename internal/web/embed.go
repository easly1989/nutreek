package web

import (
	"embed"
	"io/fs"
)

//go:embed all:templates
var templatesFS embed.FS

//go:embed all:static
var staticFS embed.FS

// Templates returns the virtual filesystem with every template.
func Templates() fs.FS {
	return templatesFS
}

// Static returns the virtual filesystem of the static assets served from
// /static/ (CSS, JS, images).
func Static() fs.FS {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		// The embed is static: when it fails it is a build-time bug.
		panic(err)
	}
	return sub
}
