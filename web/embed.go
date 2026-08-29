// Package web embeds the built UI assets so the daemon can serve them without
// external files. Until a front-end build exists, ui/dist holds a placeholder.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:ui/dist
var dist embed.FS

// Assets returns the embedded UI file tree rooted at the build output.
func Assets() fs.FS {
	sub, err := fs.Sub(dist, "ui/dist")
	if err != nil {
		// The embed path is a compile-time constant, so this cannot fail.
		panic(err)
	}
	return sub
}
