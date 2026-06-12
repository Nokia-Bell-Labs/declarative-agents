// Copyright (c) 2026 Nokia. All rights reserved.

// Package stl provides the standard tool library — shared file, build,
// and subprocess tool implementations that any agent can import.
// All tools implement core.Command / core.Builder.
package stl

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidatePath normalizes requested against the workspace root, resolves
// symlinks, and confirms the result stays inside root.
// Returns the resolved absolute path on success.
func ValidatePath(root, requested string) (string, error) {
	joined := requested
	if !filepath.IsAbs(requested) {
		joined = filepath.Join(root, requested)
	}
	joined = filepath.Clean(joined)

	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("cannot resolve workspace root: %w", err)
	}

	resolved, err := filepath.EvalSymlinks(joined)
	if err != nil {
		if os.IsNotExist(err) {
			if !pathWouldBeInside(joined, resolvedRoot) {
				return "", fmt.Errorf("path %s is outside the workspace", requested)
			}
			return "", fmt.Errorf("path not found: %s", requested)
		}
		return "", fmt.Errorf("cannot resolve path %s: %w", requested, err)
	}

	if !strings.HasPrefix(resolved, resolvedRoot+string(filepath.Separator)) && resolved != resolvedRoot {
		return "", fmt.Errorf("path %s is outside the workspace", requested)
	}

	return resolved, nil
}

// pathWouldBeInside checks whether a cleaned (but unresolved) path would
// land inside root by walking up to the deepest existing ancestor.
func pathWouldBeInside(cleaned, resolvedRoot string) bool {
	dir := cleaned
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
		resolved, err := filepath.EvalSymlinks(dir)
		if err != nil {
			continue
		}
		return strings.HasPrefix(resolved, resolvedRoot+string(filepath.Separator)) || resolved == resolvedRoot
	}
	return false
}

// RelPath returns the path of resolved relative to root, using forward
// slashes regardless of OS.
func RelPath(root, resolved string) string {
	rel, err := filepath.Rel(root, resolved)
	if err != nil {
		return filepath.ToSlash(resolved)
	}
	return filepath.ToSlash(rel)
}
