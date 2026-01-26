package controlccx

import (
	"embed"
	"io/fs"
)

// webDist contains the built web UI assets (Vite output).
//
// It is embedded into the server binary so the backend can serve UI + API from a single origin.
//go:embed web/dist
var webDist embed.FS

func WebDistFS() fs.FS {
	sub, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		return nil
	}
	return sub
}

