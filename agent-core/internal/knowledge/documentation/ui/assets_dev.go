//go:build !production

package ui

import (
	"io/fs"
	"os"
)

// Assets returns the development documentation UI asset directory.
func Assets() fs.FS {
	return os.DirFS("internal/knowledge/documentation/ui/dist")
}
