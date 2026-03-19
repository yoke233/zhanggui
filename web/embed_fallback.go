package webassets

import (
	"embed"
	"io/fs"
)

// fallbackFS contains a minimal SPA shell served as the embedded frontend.
//
//go:embed all:embedded_fallback
var fallbackFS embed.FS

// EmbeddedFrontendMode identifies which embedded frontend source is compiled.
func EmbeddedFrontendMode() string {
	return "fallback"
}

// DistFS returns a filesystem rooted at the fallback embedded frontend.
func DistFS() (fs.FS, error) {
	return fs.Sub(fallbackFS, "embedded_fallback")
}
