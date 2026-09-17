// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package declstyle

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"gopkg.in/yaml.v3"
)

// Entry classes. Adding a class is one line here plus its emitter.
const (
	classUntypedTool       = "untyped-tool"
	classProseDefaulted    = "prose-defaulted"
	classUncategorizedTool = "uncategorized-tool"
)

// typedCategories are the categories a signature is expected to cover. Boundary
// and stateful_internal joined word and response under srd051 R6.14: their
// signature names their signals and discharges the descriptive prose, while
// their effect blocks stay explicit (R6.8, R6.9), so an unsigned one is a legacy
// form to retire (GH-2110).
var typedCategories = map[string]bool{
	"word": true, "response": true, "boundary": true, "stateful_internal": true,
}

// TestDeclarationLegacyBaseline holds the line on declaration form while the
// type system migration runs. Every legacy usage in the repository is listed in
// legacy_baseline.txt; the gate fails on any usage not in it, and on any entry
// no longer present, so the baseline shrinks and never grows.
func TestDeclarationLegacyBaseline(t *testing.T) {
	found, lengths := collectLegacyEntries(t)
	baseline := loadBaseline(t, filepath.Join(thisDir(t), "legacy_baseline.txt"))

	var added []string
	for _, entry := range found {
		if !baseline[entry] {
			added = append(added, describeNewEntry(entry, lengths))
		}
	}
	seen := make(map[string]bool, len(found))
	for _, entry := range found {
		seen[entry] = true
	}
	var stale []string
	for entry := range baseline {
		if !seen[entry] {
			stale = append(stale, entry)
		}
	}
	sort.Strings(stale)

	if len(added) > 0 {
		t.Errorf("new legacy declaration forms (convert them, or add these lines "+
			"to legacy_baseline.txt with a reason):\n  %s", strings.Join(added, "\n  "))
	}
	if len(stale) > 0 {
		t.Errorf("stale legacy_baseline.txt entries (these forms are gone; delete "+
			"the lines):\n  %s", strings.Join(stale, "\n  "))
	}
}

// declarationFile is the subset of a tool declaration this gate reads. It
// decodes with plain yaml.v3 rather than importing catalog, so the gate stays
// test-only and cannot drift into depending on the production loader.
type declarationFile struct {
	Tools []struct {
		Name      string `yaml:"name"`
		Category  string `yaml:"category"`
		Signature *struct {
			Input  string   `yaml:"input"`
			Output string   `yaml:"output"`
			Emits  []string `yaml:"emits"`
		} `yaml:"signature"`
		SideEffects   []map[string]any `yaml:"side_effects"`
		Reversibility struct {
			Classification string `yaml:"classification"`
		} `yaml:"reversibility"`
		Undo struct {
			Strategy string `yaml:"strategy"`
		} `yaml:"undo"`
	} `yaml:"tools"`
}

func collectLegacyEntries(t *testing.T) ([]string, map[string]int) {
	t.Helper()
	var entries []string
	paths, err := discoverLegacyDeclarationFiles(declarationRoots(t))
	require.NoError(t, err)
	for _, path := range paths {
		entries = append(entries, fileEntries(t, path)...)
	}
	singles, err := singleImporterEntries(paths, moduleRoot(t), filepath.Dir(moduleRoot(t)))
	require.NoError(t, err)
	entries = append(entries, singles...)
	lengths, err := longFileEntries(paths, filepath.Dir(moduleRoot(t)))
	require.NoError(t, err)
	for entry := range lengths {
		entries = append(entries, entry)
	}
	sort.Strings(entries)
	return entries, lengths
}

func discoverLegacyDeclarationFiles(roots []string) ([]string, error) {
	var paths []string
	for _, root := range roots {
		if err := filepath.WalkDir(root, visitLegacyDeclarationPath(root, &paths)); err != nil {
			return nil, err
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func visitLegacyDeclarationPath(root string, paths *[]string) fs.WalkDirFunc {
	return func(path string, entry fs.DirEntry, walkErr error) error {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if walkErr != nil {
			// Generated package trees can disappear while concurrent audit
			// lanes replace them. They never contain authored declarations.
			if isGeneratedDeclarationTree(rel) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() {
			if isGeneratedDeclarationTree(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if isYAML(path) {
			*paths = append(*paths, path)
		}
		return nil
	}
}

func isGeneratedDeclarationTree(rel string) bool {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for index, part := range parts {
		switch part {
		case ".git", "build", "generated-files", "node_modules":
			return true
		case "helm":
			if index+1 < len(parts) &&
				(parts[index+1] == "dist" || parts[index+1] == "profiles") {
				return true
			}
		}
	}
	return false
}

func TestDiscoverLegacyDeclarationFilesExcludesGeneratedTrees(t *testing.T) {
	root := t.TempDir()
	canonical := filepath.Join(root, "agents", "example", "declarations.yaml")
	for _, path := range append([]string{canonical},
		filepath.Join(root, "build", "profiles", "declarations.yaml"),
		filepath.Join(root, "helm", "profiles", "declarations.yaml"),
		filepath.Join(root, "helm", "dist", "declarations.yaml"),
		filepath.Join(root, "generated-files", "declarations.yaml"),
		filepath.Join(root, "node_modules", "package", "declarations.yaml"),
	) {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte("tools: []\n"), 0o644))
	}

	paths, err := discoverLegacyDeclarationFiles([]string{root})

	require.NoError(t, err)
	require.Equal(t, []string{canonical}, paths)
}

func TestVisitLegacyDeclarationPathIgnoresGeneratedChurn(t *testing.T) {
	root := t.TempDir()
	var paths []string
	visit := visitLegacyDeclarationPath(root, &paths)
	for _, rel := range []string{
		"build/profiles",
		"helm/profiles",
		"helm/dist",
		"generated-files",
		"node_modules/package",
	} {
		err := visit(filepath.Join(root, filepath.FromSlash(rel)), nil, os.ErrNotExist)
		require.NoError(t, err, rel)
	}

	err := visit(filepath.Join(root, "agents", "missing"), nil, os.ErrNotExist)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func fileEntries(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var file declarationFile
	// A file that is not a tool declaration decodes to nothing and contributes
	// nothing; this walks far more YAML than it classifies.
	if yaml.Unmarshal(placeholderSafe(data), &file) != nil {
		return nil
	}
	rel := repoRelative(t, path)
	var entries []string
	for _, tool := range file.Tools {
		if tool.Name == "" {
			continue
		}
		switch {
		case tool.Category == "":
			// srd051 R6.15: which contract defaults apply depends on the
			// category, so a tool without one cannot be judged typed or not.
			entries = append(entries, fmt.Sprintf("%s:%s:%s", classUncategorizedTool, rel, tool.Name))
		case tool.Signature == nil:
			if typedCategories[tool.Category] {
				entries = append(entries, fmt.Sprintf("%s:%s:%s", classUntypedTool, rel, tool.Name))
			}
		case retainsDefaultedProse(tool.Category, tool.SideEffects, tool.Reversibility.Classification, tool.Undo.Strategy):
			entries = append(entries, fmt.Sprintf("%s:%s:%s", classProseDefaulted, rel, tool.Name))
		}
	}
	return entries
}

// retainsDefaultedProse reports a signed tool still carrying a block whose value
// is exactly what its category would default it to, so deleting it changes
// nothing. A block that differs is an authored override and is left alone
// (srd051 R6.10). Which blocks a signature defaults depends on the category,
// read from the one table catalog owns: a boundary tool's signature defaults
// none of them (R6.9), so its reversible/noop blocks are authored, and
// deleting them would fail the corpus audit rather than change nothing.
func retainsDefaultedProse(category string, sideEffects []map[string]any, reversibility, undo string) bool {
	defaultsSideEffects, defaultsReversibility, defaultsUndo := catalog.SignatureDischarges(category)
	if defaultsReversibility && reversibility == "reversible" {
		return true
	}
	if defaultsUndo && undo == "noop" {
		return true
	}
	return defaultsSideEffects && len(sideEffects) == 1 && fmt.Sprint(sideEffects[0]["kind"]) == "none"
}

// declarationRoots are the trees this gate classifies: the agent-core fixtures
// and every application. The shipped builtin words under agent-core/tools are
// included because GH-1971 converts them too.
func declarationRoots(t *testing.T) []string {
	t.Helper()
	module := moduleRoot(t)
	repo := filepath.Dir(module)
	return []string{
		filepath.Join(module, "testdata"),
		filepath.Join(module, "tools"),
		filepath.Join(repo, "applications"),
	}
}

func isYAML(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".yaml" || ext == ".yml"
}

func repoRelative(t *testing.T, path string) string {
	t.Helper()
	repo := filepath.Dir(moduleRoot(t))
	rel, err := filepath.Rel(repo, path)
	require.NoError(t, err)
	return filepath.ToSlash(rel)
}

func thisDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Dir(file)
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir := thisDir(t)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, parent, dir, "go.mod not found above the test directory")
		dir = parent
	}
}

func loadBaseline(t *testing.T, path string) map[string]bool {
	t.Helper()
	file, err := os.Open(path)
	require.NoError(t, err, "regenerate with: go test ./internal/declstyle -update")
	defer func() { _ = file.Close() }()
	baseline := map[string]bool{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		baseline[line] = true
	}
	require.NoError(t, scanner.Err())
	return baseline
}
