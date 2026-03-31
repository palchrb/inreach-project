// Package decoder embeds and serves the weather/avalanche decoder PWA.
package decoder

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/*
var staticFiles embed.FS

// Handler returns an http.Handler that serves the decoder PWA.
func Handler() http.Handler {
	sub, _ := fs.Sub(staticFiles, "static")
	return http.FileServer(http.FS(sub))
}
