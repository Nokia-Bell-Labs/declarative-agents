// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// repoAgentCoreRoot returns the agent-core module root, which is the parent of
// pkg/spec.
func repoAgentCoreRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	require.DirExists(t, filepath.Join(root, "tools"))
	return root
}

// declaredWordsOnDisk reads every word declared under tools/, without the
// corpus loader, so the test's expectation is independent of the code it checks.
func declaredWordsOnDisk(t *testing.T, root string) map[string]string {
	t.Helper()
	words := make(map[string]string)
	err := filepath.WalkDir(filepath.Join(root, "tools"), func(path string, entry os.DirEntry, err error) error {
		require.NoError(t, err)
		if entry.IsDir() || filepath.Ext(path) != ".yaml" {
			return nil
		}
		data, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		var file struct {
			Tools []struct {
				Name string `yaml:"name"`
			} `yaml:"tools"`
		}
		require.NoError(t, yaml.Unmarshal(data, &file), path)
		for _, tool := range file.Tools {
			if tool.Name == "" {
				continue
			}
			rel, relErr := filepath.Rel(root, path)
			require.NoError(t, relErr)
			words[tool.Name] = rel
		}
		return nil
	})
	require.NoError(t, err)
	return words
}

// TestCorpusReachesEveryDeclaredWord covers GH-1525 AC1: the corpus loader must
// see every word declared under tools/, not only the ones a non-recursive glob
// happened to reach.
func TestCorpusReachesEveryDeclaredWord(t *testing.T) {
	root := repoAgentCoreRoot(t)
	onDisk := declaredWordsOnDisk(t, root)
	require.NotEmpty(t, onDisk)

	corpus, err := LoadCorpus(root)
	require.NoError(t, err)

	var missing []string
	for name, source := range onDisk {
		if _, ok := corpus.ToolDeclarations[name]; !ok {
			missing = append(missing, name+" ("+source+")")
		}
	}
	sort.Strings(missing)
	require.Empty(t, missing, "words declared on disk but absent from the corpus")
	require.Len(t, corpus.ToolDeclarations, len(onDisk))
}

// TestCorpusResolvesIncludes covers GH-1525 AC2: the corpus loader and the
// runtime catalog loader must agree about what an includes-only file declares.
// tools/builtin/all.yaml declares no tools of its own; every word it contributes
// arrives through an include.
func TestCorpusResolvesIncludes(t *testing.T) {
	root := repoAgentCoreRoot(t)
	allPath := filepath.Join(root, "tools", "builtin", "all.yaml")

	data, err := os.ReadFile(allPath)
	require.NoError(t, err)
	var file ToolDeclFile
	require.NoError(t, yaml.Unmarshal(data, &file))
	require.Empty(t, file.Tools, "all.yaml is expected to declare no tools directly")
	require.NotEmpty(t, file.Includes, "all.yaml is expected to be an includes bundle")

	loaded, err := loadToolDeclarationsRecursive(allPath, nil)
	require.NoError(t, err)
	require.NotEmpty(t, loaded, "includes must contribute declarations")

	// A word only reachable through a nested include must be present.
	names := make(map[string]bool, len(loaded))
	for _, entry := range loaded {
		names[entry.decl.Name] = true
	}
	require.True(t, names["list_resource"], "filesystem/all.yaml include did not resolve")
	require.True(t, names["await_spans"], "otlp/all.yaml include did not resolve")
}

// TestCorpusIncludeDiamondIsNotACycle covers a regression the first
// implementation had: two branches reaching the same file is legal, and only an
// ancestor chain is a cycle.
func TestCorpusIncludeDiamondIsNotACycle(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
		return path
	}
	write("leaf.yaml", "tools:\n  - name: leaf_word\n")
	write("left.yaml", "includes: [leaf.yaml]\ntools: []\n")
	write("right.yaml", "includes: [leaf.yaml]\ntools: []\n")
	top := write("top.yaml", "includes: [left.yaml, right.yaml]\ntools: []\n")

	loaded, err := loadToolDeclarationsRecursive(top, nil)
	require.NoError(t, err, "a diamond include must not be reported as a cycle")
	require.NotEmpty(t, loaded)
}

func TestCorpusIncludeCycleIsRejected(t *testing.T) {
	dir := t.TempDir()
	aPath := filepath.Join(dir, "a.yaml")
	bPath := filepath.Join(dir, "b.yaml")
	require.NoError(t, os.WriteFile(aPath, []byte("includes: [b.yaml]\ntools: []\n"), 0o600))
	require.NoError(t, os.WriteFile(bPath, []byte("includes: [a.yaml]\ntools: []\n"), 0o600))

	_, err := loadToolDeclarationsRecursive(aPath, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "circular tool declaration include")
}

// TestUnresolvedProfileDeclarationIsReported covers GH-1525 AC3: a declaration
// path a profile names but that cannot be read must produce a named diagnostic
// rather than a silent skip, and must not fail the load. The absolute
// container-path case is the one that occurs in practice on a host checkout.
func TestUnresolvedProfileDeclarationIsReported(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	profileDir := filepath.Join(root, AgentsDir, "sample")
	require.NoError(t, os.MkdirAll(profileDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(profileDir, "profile.yaml"),
		[]byte("tool_declarations:\n  - /opt/agent-core/tools/builtin/llm/all.yaml\n"),
		0o600,
	))

	corpus, err := LoadCorpus(root, WithOptionalCorpus())
	require.NoError(t, err, "an unreadable declared path must not fail the load")
	require.Len(t, corpus.UnresolvedDeclFiles, 1)
	require.Contains(t, corpus.UnresolvedDeclFiles[0], "llm/all.yaml")

	findings := checkUnresolvedDeclarationFiles(corpus)
	require.Len(t, findings, 1)
	require.Equal(t, "warning", findings[0].Level)
	require.Contains(t, findings[0].Message, "llm/all.yaml")
}

// TestContractBaselineMatchesCorpus covers GH-1525 AC4 and AC6: the recorded
// baseline must describe exactly the words that are incomplete today, so
// mage audit passes against it with no error-level finding.
func TestContractBaselineMatchesCorpus(t *testing.T) {
	root := repoAgentCoreRoot(t)
	corpus, err := LoadCorpus(root)
	require.NoError(t, err)

	findings := checkDeclaredToolContractCompleteness(corpus)
	var errorsFound []string
	for _, finding := range findings {
		if finding.Level == "error" {
			errorsFound = append(errorsFound, finding.Message)
		}
	}
	require.Empty(t, errorsFound,
		"regenerate %s: the recorded baseline no longer matches the corpus", ContractBaselineFile)

	baseline, ok, err := loadContractBaseline(root)
	require.NoError(t, err)
	require.True(t, ok, "agent-core must ship a contract baseline")
	require.Len(t, baseline, len(IncompleteToolContracts(corpus)))
}

// TestContractBaselineRatchet covers GH-1525 AC5 in both directions: a newly
// incomplete word fails, and a word that has become complete fails until the
// baseline is ratcheted down.
func TestContractBaselineRatchet(t *testing.T) {
	root := repoAgentCoreRoot(t)
	corpus, err := LoadCorpus(root)
	require.NoError(t, err)

	gaps := IncompleteToolContracts(corpus)

	t.Run("a newly declared incomplete word is an error", func(t *testing.T) {
		scoped := cloneCorpusForBaselineTest(corpus)
		scoped.ToolDeclarations["brand_new_word"] = ToolDeclaration{Name: "brand_new_word"}

		require.Contains(t, baselineCheckNames(scoped), "tool-contract-incomplete-new")
	})

	t.Run("a word that became complete is an error until the baseline shrinks", func(t *testing.T) {
		if len(gaps) == 0 {
			finding, produced := compareContractToBaseline(
				"completed_word", completeToolDeclaration("completed_word"),
				[]string{"non_goals"}, true,
			)
			require.True(t, produced)
			require.Equal(t, "tool-contract-baseline-stale", finding.Check)
			return
		}
		scoped := cloneCorpusForBaselineTest(corpus)
		completed := gaps[0].Tool
		scoped.ToolDeclarations[completed] = completeToolDeclaration(completed)

		require.Contains(t, baselineCheckNames(scoped), "tool-contract-baseline-stale")
	})

	t.Run("a word whose missing fields changed is an error", func(t *testing.T) {
		if len(gaps) == 0 {
			partial := completeToolDeclaration("changed_word")
			partial.NonGoals = nil
			finding, produced := compareContractToBaseline(
				"changed_word", partial, []string{"goals"}, true,
			)
			require.True(t, produced)
			require.Equal(t, "tool-contract-baseline-drift", finding.Check)
			return
		}
		scoped := cloneCorpusForBaselineTest(corpus)
		changed := gaps[0].Tool
		partial := completeToolDeclaration(changed)
		partial.NonGoals = nil
		scoped.ToolDeclarations[changed] = partial

		require.Contains(t, baselineCheckNames(scoped), "tool-contract-baseline-drift")
	})
}

func baselineCheckNames(corpus *Corpus) []string {
	var checks []string
	for _, finding := range checkDeclaredToolContractCompleteness(corpus) {
		if finding.Level == "error" {
			checks = append(checks, finding.Check)
		}
	}
	return checks
}

func cloneCorpusForBaselineTest(corpus *Corpus) *Corpus {
	clone := *corpus
	clone.ToolDeclarations = make(map[string]ToolDeclaration, len(corpus.ToolDeclarations))
	for name, decl := range corpus.ToolDeclarations {
		clone.ToolDeclarations[name] = decl
	}
	return &clone
}
