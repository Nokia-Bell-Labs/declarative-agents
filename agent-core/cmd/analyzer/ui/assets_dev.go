//go:build !production

package ui

import (
	"io/fs"
	"os"
)

func Assets() fs.FS {
	return os.DirFS("cmd/analyzer/ui/dist")
}
