// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"path/filepath"
)

// charterFile identifies externally loaded evidence during reduction.
type charterFile struct {
	rel     string
	display string
}

func displayCharterPath(rootRel, rel string) string {
	if rootRel == "." || rootRel == "" {
		return rel
	}
	return filepath.ToSlash(filepath.Join(rootRel, rel))
}
