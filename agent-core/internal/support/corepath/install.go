// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package corepath resolves paths into an installed Agent Core asset root.
package corepath

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
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
// directory, and may not leave the library root the importer sits in unless it
// lands in another root (R2.4). An absolute path under InstallPrefix names
// agent-core's library: it maps through InstallRoot when one is set and is
// used as written otherwise, which is where the runtime image installs it. An
// absolute path under /opt/<name> names a declared root. Any other absolute
// path is ErrOutsideLibraryRoot.
func ImportTarget(importer, path string) (string, error) {
	if !filepath.IsAbs(path) {
		target := filepath.Join(filepath.Dir(importer), path)
		if root, inLibrary := LibraryRootOf(importer); inLibrary {
			if targetRoot, _ := LibraryRootOf(target); targetRoot != root {
				return "", fmt.Errorf("%w %q", ErrEscapesLibraryRoot, root)
			}
		}
		return target, nil
	}
	mapped, err := MapLibraryPath(path)
	if err != nil {
		return "", err
	}
	if mapped == "" {
		return "", ErrOutsideLibraryRoot
	}
	return mapped, nil
}

// Declared library roots (srd056 R2). A profile declares roots by name; the
// loader registers them for the duration of that closure's load and restores
// the previous set afterwards, so a root never leaks from one closure into
// another. --library overrides are process-scoped, like the install root.

// LibraryPrefix is the directory every library root sits under as written.
const LibraryPrefix = "/opt"

// AgentCoreRoot is the reserved name of the implicit agent-core root.
const AgentCoreRoot = "agent-core"

var libraryRootName = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

var libraries struct {
	mu        sync.RWMutex
	roots     map[string]string
	overrides map[string]string
}

// ValidateLibraryRootName rejects a root name that is not lowercase kebab or
// that claims the agent-core name (srd056 R2.1).
func ValidateLibraryRootName(name string) error {
	if name == AgentCoreRoot {
		return fmt.Errorf("library root %q is the implicit agent-core root and cannot be declared", name)
	}
	if !libraryRootName.MatchString(name) {
		return fmt.Errorf("library root %q is not a lowercase kebab name", name)
	}
	return nil
}

// SetLibraryRoots registers the declared roots of the closure being loaded,
// each name mapped to its resolved directory, and returns the previous set for
// the caller to restore.
func SetLibraryRoots(roots map[string]string) map[string]string {
	libraries.mu.Lock()
	defer libraries.mu.Unlock()
	previous := libraries.roots
	libraries.roots = cloneRoots(roots)
	return previous
}

// LibraryRoots returns the declared roots currently registered.
func LibraryRoots() map[string]string {
	libraries.mu.RLock()
	defer libraries.mu.RUnlock()
	return cloneRoots(libraries.roots)
}

// SetLibraryOverrides records --library name=path overrides, which replace a
// declared root's directory in every closure of this process (srd056 R2.2).
func SetLibraryOverrides(overrides map[string]string) {
	libraries.mu.Lock()
	defer libraries.mu.Unlock()
	libraries.overrides = cloneRoots(overrides)
}

// LibraryOverrides returns the process-scoped root overrides.
func LibraryOverrides() map[string]string {
	libraries.mu.RLock()
	defer libraries.mu.RUnlock()
	return cloneRoots(libraries.overrides)
}

// closureLoads serializes WithLibraryRoots: the registry is process-scoped, so
// two closures loading at once would otherwise see each other's roots.
var closureLoads sync.Mutex

// WithLibraryRoots runs load with one closure's declared roots in force,
// each replaced by its --library override when one is set, and restores the
// previous set afterwards (srd056 R2.2, R2.3). A request-scoped profile loaded
// while an agent runs uses it, so its rooted references resolve as they did
// when the agent's own closure loaded.
func WithLibraryRoots(declared map[string]string, load func() error) error {
	closureLoads.Lock()
	defer closureLoads.Unlock()
	overrides := LibraryOverrides()
	roots := make(map[string]string, len(declared))
	for name, directory := range declared {
		if override, ok := overrides[name]; ok {
			directory = override
		}
		roots[name] = directory
	}
	previous := SetLibraryRoots(roots)
	defer SetLibraryRoots(previous)
	return load()
}

func cloneRoots(roots map[string]string) map[string]string {
	if len(roots) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(roots))
	for name, directory := range roots {
		cloned[name] = filepath.Clean(directory)
	}
	return cloned
}

// UndeclaredRootError is an import under /opt/<name> where no root of that
// name is declared in the closure (srd056 R2.3).
type UndeclaredRootError struct{ Name string }

func (e UndeclaredRootError) Error() string {
	return fmt.Sprintf("library root %q is not declared by the profile", e.Name)
}

// ErrEscapesLibraryRoot marks a relative import in a library file that resolves
// outside its root; a library is read-only and self-contained (srd056 R2.4).
var ErrEscapesLibraryRoot = errors.New("relative import leaves its library root")

// libraryPrefixName splits /opt/<name>/rest into name and rest; ok is false
// for a path not under LibraryPrefix.
func libraryPrefixName(path string) (name, rest string, ok bool) {
	clean := filepath.ToSlash(filepath.Clean(path))
	if !strings.HasPrefix(clean, LibraryPrefix+"/") {
		return "", "", false
	}
	trimmed := strings.TrimPrefix(clean, LibraryPrefix+"/")
	name, rest, _ = strings.Cut(trimmed, "/")
	return name, rest, name != ""
}

// MapLibraryPath maps an absolute path under a library prefix to the file it
// names: the install root for agent-core, or a declared root's directory. A
// path under no prefix is returned empty with a nil error; an undeclared name
// is UndeclaredRootError.
func MapLibraryPath(path string) (string, error) {
	if UnderInstallPrefix(path) {
		if mapped := Map(path); mapped != "" {
			return mapped, nil
		}
		return filepath.Clean(path), nil
	}
	name, rest, ok := libraryPrefixName(path)
	if !ok {
		return "", nil
	}
	libraries.mu.RLock()
	directory, declared := libraries.roots[name]
	libraries.mu.RUnlock()
	if !declared {
		return "", UndeclaredRootError{Name: name}
	}
	return filepath.Join(directory, filepath.FromSlash(rest)), nil
}

// LibraryRootOf names the root a resolved file sits under: agent-core for a
// file under the install prefix or the install root's tools directory, a
// declared root's name for a file under its directory, and empty otherwise.
func LibraryRootOf(path string) (string, bool) {
	if UnderInstallPrefix(path) {
		return AgentCoreRoot, true
	}
	if root := InstallRoot(); root != "" && within(filepath.Join(root, "tools"), path) {
		return AgentCoreRoot, true
	}
	libraries.mu.RLock()
	defer libraries.mu.RUnlock()
	for name, directory := range libraries.roots {
		if within(directory, path) {
			return name, true
		}
	}
	return "", false
}

func within(dir, path string) bool {
	relative, err := filepath.Rel(dir, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
