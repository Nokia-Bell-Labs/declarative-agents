// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
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

// BuildProgramRef returns the immutable identity of one declarative program.
func BuildProgramRef(paths ProgramPaths) (core.ProgramRef, error) {
	files, err := ProgramAssetFiles(paths)
	if err != nil {
		return core.ProgramRef{}, err
	}
	hash := sha256.New()
	for _, path := range files {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return core.ProgramRef{}, fmt.Errorf("read program asset %s: %w", path, readErr)
		}
		_, _ = hash.Write([]byte(path))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(data)
		_, _ = hash.Write([]byte{0})
	}
	return core.ProgramRef{
		Profile: canonicalProgramPath(paths.Profile),
		Digest:  hex.EncodeToString(hash.Sum(nil)),
	}, nil
}

// ProgramAssetFiles returns the sorted declaration closure hashed by
// BuildProgramRef.
func ProgramAssetFiles(paths ProgramPaths) ([]string, error) {
	files := make(map[string]bool)
	addProgramPaths(files, []string{
		paths.Profile,
		paths.Machine,
	})
	addProgramPaths(files, paths.ToolSelections)
	addProgramPaths(files, paths.ToolDeclarations)
	addProgramPaths(files, paths.RESTDefinitions)
	for _, declaration := range paths.ToolDeclarations {
		if _, err := loadToolDefsRecursive(
			declaration, nil, nil,
			func(path string, _ []byte) error {
				files[path] = true
				return nil
			},
		); err != nil {
			return nil, err
		}
	}
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
