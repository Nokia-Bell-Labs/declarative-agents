//go:build !production

package ui

import (
	"io/fs"
	"os"
)

func Assets() fs.FS {
	return os.DirFS("pkg/bench/ui/dist")
}
