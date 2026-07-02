//go:build production

package ui

import (
	"embed"
	"io/fs"
)

//go:embed dist/*
var dist embed.FS

func Assets() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
