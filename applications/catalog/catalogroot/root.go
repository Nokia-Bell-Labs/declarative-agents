// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package catalogroot resolves the source root of the application catalog.
package catalogroot

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const catalogModule = "github.com/Nokia-Bell-Labs/declarative-agents/applications/catalog"

// Source describes how a catalog root was selected.
type Source string

const (
	SourceExplicit  Source = "explicit catalog root"
	SourceDiscovery Source = "repository discovery"
)

// Resolution is one absolute catalog root and its provenance.
type Resolution struct {
	Path   string
	Source Source
}

// Resolve applies the catalog-root policy to an explicit root and optional
// discovery candidates. cwd must be the process working directory captured
// before command work begins. A non-empty catalogRoot -- an application's
// declared catalog_root -- is honored as the explicit root; otherwise the
// candidates are searched for the applications/catalog source root. No
// environment variable is consulted.
func Resolve(owner, cwd, catalogRoot string, candidates ...string) (Resolution, error) {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return Resolution{}, fmt.Errorf("%s: startup working directory is empty", owner)
	}
	cwd, err := filepath.Abs(filepath.Clean(cwd))
	if err != nil {
		return Resolution{}, fmt.Errorf("%s: resolve startup working directory %q: %w", owner, cwd, err)
	}

	explicit, err := absolute(strings.TrimSpace(catalogRoot), cwd)
	if err != nil {
		return Resolution{}, invalidError(owner, catalogRoot, err)
	}
	if explicit != "" {
		return validate(owner, Resolution{Path: explicit, Source: SourceExplicit})
	}

	var attempted string
	for _, candidate := range candidates {
		path, pathErr := absolute(candidate, cwd)
		if pathErr != nil {
			continue
		}
		if attempted == "" {
			attempted = path
		}
		if isCatalog(path) {
			return Resolution{Path: path, Source: SourceDiscovery}, nil
		}
	}
	if attempted == "" {
		attempted = filepath.Join(cwd, "applications", "catalog")
	}
	return Resolution{}, fmt.Errorf(
		"%s: catalog root not found at %s; set catalog_root to the applications/catalog source root",
		owner, attempted,
	)
}

// DiscoveryCandidates returns applications/catalog beneath cwd and each
// ancestor, nearest first. It never probes a top-level agent-profiles path.
func DiscoveryCandidates(cwd string) []string {
	dir := filepath.Clean(cwd)
	var candidates []string
	for {
		candidates = append(candidates, filepath.Join(dir, "applications", "catalog"))
		parent := filepath.Dir(dir)
		if parent == dir {
			return candidates
		}
		dir = parent
	}
}

// AgentsRoot returns the reusable agent-block directory.
func (r Resolution) AgentsRoot() string {
	return filepath.Join(r.Path, "agents")
}

// ConformanceRoot returns the catalog-owned conformance fixture directory.
func (r Resolution) ConformanceRoot() string {
	return filepath.Join(r.Path, "testdata", "conformance")
}

func validate(owner string, resolution Resolution) (Resolution, error) {
	if isCatalog(resolution.Path) {
		return resolution, nil
	}
	return Resolution{}, invalidError(owner, resolution.Path, nil)
}

func invalidError(owner, path string, cause error) error {
	message := fmt.Sprintf(
		"%s: catalog_root %s is an invalid catalog root; set catalog_root to the applications/catalog source root",
		owner, path,
	)
	if cause != nil {
		return fmt.Errorf("%s: %w", message, cause)
	}
	return fmt.Errorf("%s", message)
}

func absolute(value, cwd string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(cwd, value)
	}
	return filepath.Abs(filepath.Clean(value))
}

func isCatalog(root string) bool {
	if root == "" {
		return false
	}
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return false
	}
	fields := strings.Fields(string(data))
	return len(fields) >= 2 && fields[0] == "module" && fields[1] == catalogModule &&
		isDir(filepath.Join(root, "agents"))
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
