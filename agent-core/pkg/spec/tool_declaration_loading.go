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
	readable, unresolved := partitionDeclarationFiles(declFiles, requiredSet)

	// Declarations record an absolute source, so the root must be absolute too
	// for the relative form findings quote to come out clean.
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		absRoot = rootDir
	}

	decls := make(map[string]ToolDeclaration)
	loaded, err := loadInto(decls, readable, absRoot)
	if err != nil {
		return nil, nil, err
	}

	// A word that runs a nested machine names that machine's vocabulary in its
	// own config, which is the only place those declarations are written down.
	// They cannot be in the first pass: the file list is built before any
	// declaration is parsed, so the corpus reads what it just loaded and
	// follows what that names (GH-2330).
	nested := configNamedDeclarationFiles(loaded, rootDir, seenPaths(readable))
	if _, err := loadInto(decls, nested, absRoot); err != nil {
		return nil, nil, err
	}

	sort.Strings(unresolved)
	return decls, unresolved, nil
}

// partitionDeclarationFiles splits the candidate files into the readable ones
// and the named-but-missing ones. A path a profile named explicitly is the one
// an operator can get wrong, so an unreadable one is reported rather than
// skipped (GH-1525 R3).
func partitionDeclarationFiles(declFiles []string, requiredSet map[string]bool) (readable, unresolved []string) {
	for _, path := range declFiles {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if requiredSet[path] {
				unresolved = append(unresolved, path)
			}
			continue
		}
		readable = append(readable, path)
	}
	return readable, unresolved
}

// loadInto parses paths and folds their words into decls, returning what it
// loaded so a caller can follow what those declarations themselves name.
func loadInto(
	decls map[string]ToolDeclaration, paths []string, absRoot string,
) ([]catalog.ToolDef, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	loaded, err := catalog.LoadToolDeclarationsWithOptions(
		paths,
		catalog.LoadOptions{
			TolerateNonToolFiles: true,
			ExpandEnv:            false,
			KeepConfigFiles:      true,
		},
		nil,
	)
	if err != nil {
		return nil, err
	}
	mergeToolDeclarations(decls, loaded, absRoot)
	return loaded, nil
}

func seenPaths(paths []string) map[string]bool {
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		seen[filepath.Clean(path)] = true
	}
	return seen
}

// configNamedDeclarationFiles returns the declaration files loaded words name
// in their own config, by the *_tool_declarations convention the nested-machine
// words follow. Paths resolve the way a profile's do: an installed core path
// maps onto the configured agent-core root, and a relative one resolves
// against the module root, which is what the runtime passes as --directory.
// Files already loaded, and files that do not exist, are skipped: a config
// naming a path this corpus cannot see is reported by the unresolved-path
// check rather than failing the load.
func configNamedDeclarationFiles(loaded []catalog.ToolDef, rootDir string, seen map[string]bool) []string {
	var files []string
	for _, def := range loaded {
		for key, value := range def.Config {
			if !strings.HasSuffix(key, "_tool_declarations") {
				continue
			}
			for _, named := range configStringSlice(value) {
				path := filepath.Clean(resolveProfilePath(rootDir, named))
				if seen[path] {
					continue
				}
				if _, err := os.Stat(path); err != nil {
					continue
				}
				seen[path] = true
				files = append(files, path)
			}
		}
	}
	sort.Strings(files)
	return files
}

func configStringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
			out = append(out, text)
		}
	}
	return out
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
		// builtin.yaml and declarations.yaml are the conventional names an
		// agent directory ships its own words under. A directory with a
		// machine but no profile.yaml -- a family fragment another profile
		// composes -- has nothing to name them, so the corpus reads what the
		// directory ships or sees a family that selects undeclared words
		// (GH-2330).
		for _, conventional := range []string{"builtin.yaml", "declarations.yaml"} {
			path := filepath.Join(pd.Dir, conventional)
			if _, err := os.Stat(path); err == nil {
				declFiles = append(declFiles, path)
			}
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
