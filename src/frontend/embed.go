package frontend

import (
	"embed"
	"io/fs"
)

//go:embed dist/*
var Dist embed.FS

// Assets returns the FS rooted at dist/ for the Wails asset server.
func Assets() (fs.FS, error) {
	return fs.Sub(Dist, "dist")
}
