// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package corepath resolves paths into an installed Agent Core asset root.
package corepath

import (
	"errors"
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
	if root == "" || !UnderInstallPrefix(path) {
		return ""
	}
	rel := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(path)), InstallPrefix)
	rel = strings.TrimPrefix(rel, "/")
	return filepath.Join(root, filepath.FromSlash(rel))
}

// UnderInstallPrefix reports whether path names InstallPrefix or a file below
// it, the agent-core library root.
func UnderInstallPrefix(path string) bool {
	clean := filepath.ToSlash(filepath.Clean(path))
	return clean == InstallPrefix || strings.HasPrefix(clean, InstallPrefix+"/")
}

// ErrOutsideLibraryRoot marks an absolute import path that names no library
// root; the caller reports it with its own unit and file.
var ErrOutsideLibraryRoot = errors.New("absolute import path is not under a library root")

// ImportTarget resolves a path a declaration at importer imports or
// instantiates (srd056 R1). A relative path resolves against the importer's
// directory. An absolute path under InstallPrefix names agent-core's library:
// it maps through InstallRoot when one is set and is used as written otherwise,
// which is where the runtime image installs it. Any other absolute path is
// ErrOutsideLibraryRoot.
func ImportTarget(importer, path string) (string, error) {
	if !filepath.IsAbs(path) {
		return filepath.Join(filepath.Dir(importer), path), nil
	}
	if !UnderInstallPrefix(path) {
		return "", ErrOutsideLibraryRoot
	}
	if mapped := Map(path); mapped != "" {
		return mapped, nil
	}
	return filepath.Clean(path), nil
}
