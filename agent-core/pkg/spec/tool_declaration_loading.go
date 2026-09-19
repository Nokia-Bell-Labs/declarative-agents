// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package spec

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
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
	var readable []string
	for _, path := range declFiles {
		if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
			if requiredSet[path] {
				unresolved = append(unresolved, path)
			}
			continue
		}
		readable = append(readable, path)
	}
	loaded, err := catalog.LoadToolDeclarationsWithOptions(
		readable,
		catalog.LoadOptions{
			TolerateNonToolFiles: true,
			ExpandEnv:            false,
			KeepConfigFiles:      true,
		},
		nil,
	)
	if err != nil {
		return nil, nil, err
	}
	mergeToolDeclarations(decls, loaded, absRoot)

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
	// Traversed rather than globbed: shipped words live in subdirectories, and a
	// non-recursive glob left a third of the vocabulary outside the audited
	// corpus while the runtime loaded it (GH-1525). Units ship words too (GH-2180).
	for _, dir := range []string{"builtin", "exec", "units"} {
		declFiles = append(declFiles, yamlFilesUnderDir(filepath.Join(rootDir, "tools", dir))...)
	}

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
func mergeToolDeclarations(
	decls map[string]ToolDeclaration, loaded []catalog.ToolDef, absRoot string,
) {
	for _, def := range loaded {
		source := def.DeclarationSource().Path
		relPath, relErr := filepath.Rel(absRoot, source)
		if relErr != nil || relPath == "" || strings.HasPrefix(relPath, "..") {
			relPath = source
		}
		td := toolDeclarationFromDef(def)
		td.SourceFile = relPath
		if existing, ok := decls[td.Name]; ok && keepExistingToolDeclaration(existing, td) {
			continue
		}
		decls[td.Name] = td
	}
}

func keepExistingToolDeclaration(existing, candidate ToolDeclaration) bool {
	return isAgentLocalToolDeclaration(existing.SourceFile) && !isAgentLocalToolDeclaration(candidate.SourceFile)
}

func isAgentLocalToolDeclaration(sourceFile string) bool {
	path := filepath.ToSlash(sourceFile)
	return strings.HasPrefix(path, "agents/") || strings.Contains(path, "/agents/")
}
