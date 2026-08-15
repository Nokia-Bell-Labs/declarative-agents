// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package spec

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

func discoverAndParseToolDeclarations(rootDir string) (map[string]ToolDeclaration, []string, error) {
	declFiles, requiredSet := toolDeclarationFiles(rootDir)

	// Declarations record an absolute source, so the root must be absolute too
	// for the relative form findings quote to come out clean.
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		absRoot = rootDir
	}

	decls := make(map[string]ToolDeclaration)
	var unresolved []string
	for _, path := range declFiles {
		loaded, loadErr := loadToolDeclarationsRecursive(path, nil)
		if loadErr != nil {
			if errors.Is(loadErr, os.ErrNotExist) {
				if requiredSet[path] {
					unresolved = append(unresolved, path)
				}
				continue
			}
			return nil, nil, loadErr
		}
		mergeToolDeclarations(decls, loaded, absRoot)
	}

	sort.Strings(unresolved)
	return decls, unresolved, nil
}

// toolDeclarationFiles lists every declaration file to load, and the subset a
// profile named explicitly. A named path is the one an operator can get wrong,
// so an unreadable one is reported rather than skipped (GH-1525 R3).
func toolDeclarationFiles(rootDir string) ([]string, map[string]bool) {
	declFiles := []string{
		filepath.Join(rootDir, "tools", "builtin.yaml"),
		filepath.Join(rootDir, "tools", "exec.yaml"),
	}
	// Traversed rather than globbed: shipped words live in subdirectories
	// (tools/builtin/rest, tools/builtin/otlp, tools/exec/git), and a
	// non-recursive glob left a third of the vocabulary outside the audited
	// corpus while the runtime loaded it (GH-1525).
	declFiles = append(declFiles, yamlFilesUnderDir(filepath.Join(rootDir, "tools", "builtin"))...)
	declFiles = append(declFiles, yamlFilesUnderDir(filepath.Join(rootDir, "tools", "exec"))...)

	requiredSet := make(map[string]bool)
	for _, pd := range collectProfileDirs(resolveProfileAssetsRoot(rootDir)) {
		override := filepath.Join(pd.Dir, "builtin.yaml")
		if _, err := os.Stat(override); err == nil {
			declFiles = append(declFiles, override)
		}
		declFiles = append(declFiles, yamlFilesInDir(filepath.Join(pd.Dir, "llm"))...)
		named := declarationFilesFromProfile(filepath.Join(pd.Dir, "profile.yaml"))
		declFiles = append(declFiles, named...)
		for _, path := range named {
			requiredSet[path] = true
		}
	}
	return declFiles, requiredSet
}

// mergeToolDeclarations folds loaded declarations into decls, recording each
// word's own source file relative to the corpus root.
func mergeToolDeclarations(decls map[string]ToolDeclaration, loaded []loadedToolDeclaration, absRoot string) {
	for _, entry := range loaded {
		relPath, relErr := filepath.Rel(absRoot, entry.sourceFile)
		if relErr != nil || relPath == "" || strings.HasPrefix(relPath, "..") {
			relPath = entry.sourceFile
		}
		td := entry.decl
		td.SourceFile = relPath
		if existing, ok := decls[td.Name]; ok && keepExistingToolDeclaration(existing, td) {
			continue
		}
		decls[td.Name] = td
	}
}

// loadedToolDeclaration pairs a declaration with the file it was actually read
// from, so an included word reports its own file rather than the includer's.
type loadedToolDeclaration struct {
	decl       ToolDeclaration
	sourceFile string
}

// loadToolDeclarationsRecursive resolves a declaration file and its includes,
// mirroring internal/tools/catalog's loadToolDefsRecursive. Includes resolve
// against the including file's directory.
//
// stack holds the current ancestor chain, not every file seen. Two includes
// reaching the same file by different routes is a diamond, which is legal and
// common here -- tools/builtin/all.yaml reaches write.yaml through more than one
// branch -- while a file that includes an ancestor of itself is a cycle.
func loadToolDeclarationsRecursive(path string, stack map[string]bool) ([]loadedToolDeclaration, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve tool declarations %s: %w", path, err)
	}
	if stack == nil {
		stack = make(map[string]bool)
	}
	if stack[abs] {
		return nil, fmt.Errorf("circular tool declaration include: %s", abs)
	}
	stack[abs] = true
	defer delete(stack, abs)

	data, err := os.ReadFile(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("read tool declarations %s: %w", path, os.ErrNotExist)
		}
		return nil, fmt.Errorf("read tool declarations %s: %w", path, err)
	}
	var file ToolDeclFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse tool declarations %s: %w", path, err)
	}

	loaded, err := resolveDeclarationIncludes(file.Includes, abs, stack)
	if err != nil {
		return nil, err
	}
	for _, td := range file.Tools {
		loaded = append(loaded, loadedToolDeclaration{decl: td, sourceFile: abs})
	}
	return loaded, nil
}

// resolveDeclarationIncludes loads a file's includes against its own directory,
// in declaration order. An include that does not exist is skipped: unlike a path
// a profile named, an include is an internal reference and a missing one is
// reported by the file that owns it.
func resolveDeclarationIncludes(includes []string, from string, stack map[string]bool) ([]loadedToolDeclaration, error) {
	var loaded []loadedToolDeclaration
	dir := filepath.Dir(from)
	for _, inc := range includes {
		incPath := inc
		if !filepath.IsAbs(incPath) {
			incPath = filepath.Join(dir, incPath)
		}
		included, err := loadToolDeclarationsRecursive(incPath, stack)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("include %s from %s: %w", inc, from, err)
		}
		loaded = append(loaded, included...)
	}
	return loaded, nil
}

func keepExistingToolDeclaration(existing, candidate ToolDeclaration) bool {
	return isAgentLocalToolDeclaration(existing.SourceFile) && !isAgentLocalToolDeclaration(candidate.SourceFile)
}

func isAgentLocalToolDeclaration(sourceFile string) bool {
	path := filepath.ToSlash(sourceFile)
	return strings.HasPrefix(path, "agents/") || strings.Contains(path, "/agents/")
}
