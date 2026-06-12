//go:build !production

package ui

import (
	"io/fs"
	"os"
)

func Assets() fs.FS {
	return os.DirFS("internal/evaluation/bench/ui/dist")
}
