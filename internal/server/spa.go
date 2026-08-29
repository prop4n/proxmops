package server

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// spaHandler serves the embedded single-page app: real files are served
// directly, and any other path falls back to index.html so client-side routes
// (e.g. /login) resolve on a full page load. If no build is embedded yet, it
// returns 404.
func spaHandler(assets fs.FS) http.Handler {
	fileServer := http.FileServerFS(assets)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		if _, err := fs.Stat(assets, name); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		// Unknown path: serve the SPA entry point if it exists.
		index, err := fs.ReadFile(assets, "index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(index)
	})
}
