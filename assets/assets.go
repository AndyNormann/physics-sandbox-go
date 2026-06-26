// Package assets embeds the static frontend files (compiled CSS and the
// canvas/JS client) so the server ships as a single self-contained binary.
package assets

import (
	"embed"
	"io/fs"
)

//go:embed static
var embedded embed.FS

// Static returns a filesystem rooted at the static/ directory.
func Static() fs.FS {
	sub, err := fs.Sub(embedded, "static")
	if err != nil {
		panic(err)
	}
	return sub
}
