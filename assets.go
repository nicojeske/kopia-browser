// Package assets embeds the web templates and static files so the binary is
// fully self-contained (no runtime file paths). It lives at the module root so
// the //go:embed directives can reach the web/ directory while keeping the
// documented layout (web/templates, web/static) intact.
package assets

import (
	"embed"
	"io/fs"
)

//go:embed web/templates/*.html
var templatesFS embed.FS

//go:embed all:web/static
var staticFS embed.FS

// Templates returns an fs.FS rooted at web/templates.
func Templates() fs.FS {
	sub, err := fs.Sub(templatesFS, "web/templates")
	if err != nil {
		panic(err) // directive guarantees the subtree exists
	}
	return sub
}

// Static returns an fs.FS rooted at web/static, suitable for http.FileServerFS.
func Static() fs.FS {
	sub, err := fs.Sub(staticFS, "web/static")
	if err != nil {
		panic(err)
	}
	return sub
}
