// Copyright (c) 2026 Nokia. All rights reserved.

// Package catalogroot resolves the source root of the application catalog.
package catalogroot

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// Env is the canonical catalog source-root input.
	Env = "AGENT_CATALOG_ROOT"
	// LegacyEnv is the Release 99.0 compatibility alias for Env.
	LegacyEnv = "AGENT_PROFILES_ROOT"

	catalogModule = "github.com/Nokia-Bell-Labs/declarative-agents/applications/catalog"
)

// Source describes how a catalog root was selected.
type Source string

const (
	SourceCanonical Source = Env
	SourceLegacy    Source = LegacyEnv
	SourceDiscovery Source = "repository discovery"
)

// Resolution is one absolute catalog root and its provenance.
type Resolution struct {
	Path       string
	Source     Source
	Deprecated bool
}

// Resolve applies the catalog-root policy to an environment snapshot and
// optional discovery candidates. cwd must be the process working directory
// captured before command work begins.
func Resolve(owner, cwd, canonical, legacy string, candidates ...string) (Resolution, error) {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return Resolution{}, fmt.Errorf("%s: startup working directory is empty", owner)
	}
	cwd, err := filepath.Abs(filepath.Clean(cwd))
	if err != nil {
		return Resolution{}, fmt.Errorf("%s: resolve startup working directory %q: %w", owner, cwd, err)
	}

	canonical = strings.TrimSpace(canonical)
	legacy = strings.TrimSpace(legacy)
	canonicalPath, err := absolute(canonical, cwd)
	if err != nil {
		return Resolution{}, invalidError(owner, Env, canonical, err)
	}
	legacyPath, err := absolute(legacy, cwd)
	if err != nil {
		return Resolution{}, invalidError(owner, LegacyEnv, legacy, err)
	}
	if canonicalPath != "" && legacyPath != "" && canonicalPath != legacyPath {
		return Resolution{}, fmt.Errorf(
			"%s: conflicting catalog roots: %s=%s and deprecated %s=%s resolve to different paths",
			owner, Env, canonicalPath, LegacyEnv, legacyPath,
		)
	}

	switch {
	case canonicalPath != "":
		return validate(owner, Resolution{
			Path: canonicalPath, Source: SourceCanonical, Deprecated: legacyPath != "",
		})
	case legacyPath != "":
		return validate(owner, Resolution{
			Path: legacyPath, Source: SourceLegacy, Deprecated: true,
		})
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
		"%s: catalog root not found at %s; set %s to the applications/catalog source root",
		owner, attempted, Env,
	)
}

// ResolveFromEnvironment snapshots the process CWD and both environment
// inputs, then performs repository discovery for applications/catalog.
func ResolveFromEnvironment(owner string) (Resolution, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return Resolution{}, fmt.Errorf("%s: resolve startup working directory: %w", owner, err)
	}
	return Resolve(owner, cwd, os.Getenv(Env), os.Getenv(LegacyEnv), DiscoveryCandidates(cwd)...)
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

// Deprecation returns the compatibility diagnostic, if one is required.
func (r Resolution) Deprecation() string {
	if !r.Deprecated {
		return ""
	}
	return fmt.Sprintf("%s is deprecated after Release 99.0; set %s", LegacyEnv, Env)
}

func validate(owner string, resolution Resolution) (Resolution, error) {
	if isCatalog(resolution.Path) {
		return resolution, nil
	}
	return Resolution{}, invalidError(owner, string(resolution.Source), resolution.Path, nil)
}

func invalidError(owner, input, path string, cause error) error {
	message := fmt.Sprintf(
		"%s: %s resolves to invalid catalog root %s; set %s to the applications/catalog source root",
		owner, input, path, Env,
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
