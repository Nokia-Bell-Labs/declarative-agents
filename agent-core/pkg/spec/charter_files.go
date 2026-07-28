// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"path/filepath"
	"strings"
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

func includedByGlob(path string, include []string) bool {
	if len(include) == 0 {
		return true
	}
	for _, pattern := range include {
		if matchCharterGlob(pattern, path) {
			return true
		}
	}
	return false
}

func excludedByGlob(path string, exclude []string) bool {
	for _, pattern := range exclude {
		if matchCharterGlob(pattern, path) {
			return true
		}
	}
	return false
}

func matchCharterGlob(pattern, path string) bool {
	pattern = filepath.ToSlash(filepath.Clean(strings.TrimSpace(pattern)))
	path = filepath.ToSlash(filepath.Clean(path))
	if pattern == "." || pattern == "" {
		return false
	}
	return matchGlobParts(strings.Split(pattern, "/"), strings.Split(path, "/"))
}

func matchGlobParts(pattern, path []string) bool {
	if len(pattern) == 0 {
		return len(path) == 0
	}
	if pattern[0] == "**" {
		if matchGlobParts(pattern[1:], path) {
			return true
		}
		return len(path) > 0 && matchGlobParts(pattern, path[1:])
	}
	if len(path) == 0 {
		return false
	}
	matched, err := filepath.Match(pattern[0], path[0])
	if err != nil || !matched {
		return false
	}
	return matchGlobParts(pattern[1:], path[1:])
}
