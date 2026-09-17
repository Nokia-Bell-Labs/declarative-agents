// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"path/filepath"
	"sort"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// ProgramPaths identifies every declared file root that makes up one agent
// program. Declaration includes and config-directory files are discovered from
// these roots before the program digest is built.
type ProgramPaths struct {
	Profile          string
	Machine          string
	ToolSelections   []string
	ToolDeclarations []string
	ToolConfigDirs   []string
	RESTDefinitions  []string
	RESTConfigDirs   []string
}

// BuildProgramRefFromAssets hashes the immutable bytes captured while loading
// a declaration closure.
func BuildProgramRefFromAssets(profile string, assets map[string][]byte) core.ProgramRef {
	canonical := make(map[string][]byte, len(assets))
	for path, data := range assets {
		path = canonicalProgramPath(path)
		canonical[path] = data
	}
	files := make([]string, 0, len(canonical))
	for path := range canonical {
		files = append(files, path)
	}
	sort.Strings(files)
	hash := sha256.New()
	for _, path := range files {
		_, _ = hash.Write([]byte(path))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(canonical[path])
		_, _ = hash.Write([]byte{0})
	}
	return core.ProgramRef{
		Profile: canonicalProgramPath(profile),
		Digest:  hex.EncodeToString(hash.Sum(nil)),
	}
}

// ProgramAssetFilesFromVisited returns the program assets after declaration
// includes have already been resolved by the loader: the declared roots, every
// visited declaration, and every file under the declared config directories.
func ProgramAssetFilesFromVisited(paths ProgramPaths, visited []string) ([]string, error) {
	files := make(map[string]bool)
	addProgramPaths(files, []string{paths.Profile, paths.Machine})
	addProgramPaths(files, paths.ToolSelections)
	addProgramPaths(files, paths.ToolDeclarations)
	addProgramPaths(files, paths.RESTDefinitions)
	addProgramPaths(files, visited)
	for _, dir := range append(
		append([]string(nil), paths.ToolConfigDirs...), paths.RESTConfigDirs...,
	) {
		if err := addProgramDirectory(files, dir); err != nil {
			return nil, err
		}
	}
	result := make([]string, 0, len(files))
	for path := range files {
		result = append(result, path)
	}
	sort.Strings(result)
	return result, nil
}

func addProgramPaths(files map[string]bool, paths []string) {
	for _, path := range paths {
		if path != "" {
			files[canonicalProgramPath(path)] = true
		}
	}
}

func addProgramDirectory(files map[string]bool, dir string) error {
	return filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && entry.Type().IsRegular() {
			files[canonicalProgramPath(path)] = true
		}
		return nil
	})
}

func canonicalProgramPath(path string) string {
	if path == "" {
		return ""
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(absolute)
}
