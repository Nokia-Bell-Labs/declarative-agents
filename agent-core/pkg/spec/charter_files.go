// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// charterFile is shared target-discovery evidence for the charter kinds that
// still consume complete files. grep_check deliberately bypasses this path and
// receives only rg match events.
type charterFile struct {
	abs     string
	rel     string
	display string
}

func charterRoot(targetDir, root string) (string, string) {
	if root == "" {
		root = "."
	}
	if filepath.IsAbs(root) {
		return filepath.Clean(root), "."
	}
	return filepath.Join(targetDir, root), filepath.ToSlash(filepath.Clean(root))
}

func charterFiles(root, rootRel string, include, exclude []string) ([]charterFile, error) {
	var files []charterFile
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		display := displayCharterPath(rootRel, rel)
		if !includedByGlob(rel, include) || excludedByGlob(rel, exclude) {
			return nil
		}
		files = append(files, charterFile{abs: path, rel: rel, display: display})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discover target files: %w", err)
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].display < files[j].display
	})
	return files, nil
}

func narrowCharterFiles(root, rootRel string, baseFiles []charterFile, include, exclude []string) ([]charterFile, error) {
	if len(include) == 0 && len(exclude) == 0 {
		return append([]charterFile(nil), baseFiles...), nil
	}
	return charterFiles(root, rootRel, include, exclude)
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
