// Copyright (c) 2026 Nokia. All rights reserved.

// Package corepath resolves paths into an installed Agent Core asset root.
package corepath

import (
	"path/filepath"
	"strings"
	"sync"
)

// InstallPrefix is the canonical runtime image location for Agent Core assets.
const InstallPrefix = "/opt/agent-core"

var installRoot struct {
	mu sync.RWMutex
	v  string
}

// SetInstallRoot maps InstallPrefix references to root. Leave root empty when
// the runtime provides the canonical absolute paths directly.
func SetInstallRoot(root string) {
	installRoot.mu.Lock()
	defer installRoot.mu.Unlock()
	installRoot.v = strings.TrimSpace(root)
}

// InstallRoot returns the configured development or mounted asset root.
func InstallRoot() string {
	installRoot.mu.RLock()
	defer installRoot.mu.RUnlock()
	return installRoot.v
}

// Map maps a path under InstallPrefix into InstallRoot. It returns an empty
// string when no override is configured or the path is outside the prefix.
func Map(path string) string {
	root := InstallRoot()
	if root == "" {
		return ""
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	if clean != InstallPrefix && !strings.HasPrefix(clean, InstallPrefix+"/") {
		return ""
	}
	rel := strings.TrimPrefix(clean, InstallPrefix)
	rel = strings.TrimPrefix(rel, "/")
	return filepath.Join(root, filepath.FromSlash(rel))
}
