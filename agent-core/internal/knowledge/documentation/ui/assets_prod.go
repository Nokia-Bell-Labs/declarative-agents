//go:build production

package ui

import (
	"embed"
	"io/fs"
)

//go:embed dist/*
var dist embed.FS

// Assets returns embedded production documentation UI assets.
func Assets() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
