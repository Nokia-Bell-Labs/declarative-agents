// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReduceRefSearchUsesExternalTargetAndBibTeXEvidence(t *testing.T) {
	charter := grepCharter("refs", CharterCheck{
		ID: "citations", Kind: "ref_check", Severity: "error",
		Include: []string{"*.md"},
		Refs:    map[string]any{"file": "references.bib", "format": "bibtex_keys"},
		Extract: map[string]any{"regex": `@([A-Za-z0-9_-]+)`, "group": 1},
	})
	plans, err := BuildRefSearchPlans(t.TempDir(), []Charter{charter})
	require.NoError(t, err)
	require.Len(t, plans, 1)
	output := refTargetMarker + "\n" +
		rgMatchFixture("paper.md", 2, "See @Known and @Missing.\n") +
		refFileMarker + "\n@article{Known,\n title={Known}\n}\n" +
		refDirMarker

	findings, err := ReduceRefSearch(plans[0], output)

	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, "paper.md", findings[0].File)
	assert.Equal(t, 2, findings[0].Line)
	assert.Contains(t, findings[0].Message, "Missing")
}

func TestExecuteRefChecksResolvedInlineReferencesPass(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "paper.md", "See @known and @also-known.\n")
	charter := refCharter("citation-suite", CharterCheck{
		ID:       "citations-resolve",
		Kind:     "ref_check",
		Severity: "error",
		Include:  []string{"*.md"},
		Refs:     map[string]any{"values": []any{"known", "also-known"}},
		Extract:  map[string]any{"regex": `@([A-Za-z0-9:_-]+)`},
	})

	findings, err := executeRefFixtures(t, root, []Charter{charter})

	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestExecuteRefChecksReportsMissingReferenceWithProvenance(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "paper.md", "See @known.\nBut @missing is absent.\n")
	charter := refCharter("citation-suite", CharterCheck{
		ID:       "citations-resolve",
		Kind:     "ref_check",
		Severity: "warning",
		Include:  []string{"*.md"},
		Refs:     map[string]any{"values": []any{"known"}},
		Extract:  map[string]any{"regex": `@([A-Za-z0-9:_-]+)`},
	})

	findings, err := executeRefFixtures(t, root, []Charter{charter})

	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, "warning", findings[0].Level)
	assert.Equal(t, "citation-suite", findings[0].SuiteID)
	assert.Equal(t, "citations-resolve", findings[0].CheckID)
	assert.Equal(t, "ref_check", findings[0].Kind)
	assert.Equal(t, "paper.md", findings[0].File)
	assert.Equal(t, 2, findings[0].Line)
	assert.Contains(t, findings[0].Message, "missing")
}

func TestExecuteRefChecksResolvesPathReferencesAgainstDirectory(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "manifest.txt", "artifact=results/out.json\nartifact=results/missing.json\n")
	writeTargetFile(t, root, "artifacts/results/out.json", "{}\n")
	charter := refCharter("artifact-suite", CharterCheck{
		ID:       "artifacts-exist",
		Kind:     "ref_check",
		Severity: "error",
		Include:  []string{"manifest.txt"},
		Refs:     map[string]any{"directory": "artifacts"},
		Extract:  map[string]any{"regex": `artifact=([^\s]+)`},
	})

	findings, err := executeRefFixtures(t, root, []Charter{charter})

	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, "manifest.txt", findings[0].File)
	assert.Equal(t, 2, findings[0].Line)
	assert.Contains(t, findings[0].Message, "results/missing.json")
}

func TestExecuteRefChecksLoadsBibtexKeys(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "paper.md", "Cite @Known2026 and @Missing2026.\n")
	writeTargetFile(t, root, "references.bib", "@article{Known2026,\n  title = {Known}\n}\n")
	charter := refCharter("bib-suite", CharterCheck{
		ID:       "bib-citations",
		Kind:     "ref_check",
		Severity: "error",
		Include:  []string{"*.md"},
		Refs:     map[string]any{"file": "references.bib", "format": "bibtex_keys"},
		Extract:  map[string]any{"regex": `@([A-Za-z0-9:_-]+)`},
	})

	findings, err := executeRefFixtures(t, root, []Charter{charter})

	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0].Message, "Missing2026")
}

func TestExecuteRefChecksAllowMissingSuppressesNoReferenceFinding(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "paper.md", "No citations here.\n")
	charter := refCharter("citation-suite", CharterCheck{
		ID:           "citations-resolve",
		Kind:         "ref_check",
		Severity:     "error",
		Include:      []string{"*.md"},
		Refs:         map[string]any{"values": []any{"known"}},
		Extract:      map[string]any{"regex": `@([A-Za-z0-9:_-]+)`},
		AllowMissing: true,
	})

	findings, err := executeRefFixtures(t, root, []Charter{charter})

	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestExecuteRefChecksSortsFindingsDeterministically(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "z.md", "@missing-z\n")
	writeTargetFile(t, root, "a.md", "@missing-a\n")
	charters := []Charter{
		refCharter("suite-b", CharterCheck{ID: "refs", Kind: "ref_check", Severity: "warning", Include: []string{"*.md"}, Refs: map[string]any{"values": []any{"known"}}, Extract: map[string]any{"regex": `@([A-Za-z0-9:_-]+)`}}),
		refCharter("suite-a", CharterCheck{ID: "refs", Kind: "ref_check", Severity: "warning", Include: []string{"*.md"}, Refs: map[string]any{"values": []any{"known"}}, Extract: map[string]any{"regex": `@([A-Za-z0-9:_-]+)`}}),
	}

	findings, err := executeRefFixtures(t, root, charters)

	require.NoError(t, err)
	requireDeterministicCharterOrder(t, findings, ".md")
}

func TestExecuteRefChecksNoReferencesFoundIsFindingByDefault(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "paper.md", "No citations here.\n")
	charter := refCharter("citation-suite", CharterCheck{
		ID:       "citations-resolve",
		Kind:     "ref_check",
		Severity: "error",
		Include:  []string{"*.md"},
		Refs:     map[string]any{"values": []any{"known"}},
		Extract:  map[string]any{"regex": `@([A-Za-z0-9:_-]+)`},
	})

	findings, err := executeRefFixtures(t, root, []Charter{charter})

	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Empty(t, findings[0].File)
	assert.Contains(t, findings[0].Message, "no references found")
}

func executeRefFixtures(t *testing.T, root string, charters []Charter) ([]Finding, error) {
	t.Helper()
	plans, err := BuildRefSearchPlans(root, charters)
	if err != nil {
		return nil, err
	}
	var findings []Finding
	planIndex := 0
	for _, charter := range charters {
		for _, check := range charter.Checks {
			if check.Kind != "ref_check" {
				continue
			}
			plan := plans[planIndex]
			planIndex++
			extractor, err := refExtractor(check)
			if err != nil {
				return nil, err
			}
			var target strings.Builder
			scanRoot := plan.Path
			if !filepath.IsAbs(scanRoot) {
				scanRoot = filepath.Join(root, scanRoot)
			}
			include, exclude := charter.Target.Include, charter.Target.Exclude
			if len(check.Include) > 0 || len(check.Exclude) > 0 {
				include, exclude = check.Include, check.Exclude
			}
			err = filepath.WalkDir(scanRoot, func(path string, entry os.DirEntry, err error) error {
				if err != nil || entry.IsDir() {
					return err
				}
				rel, err := filepath.Rel(scanRoot, path)
				if err != nil {
					return err
				}
				rel = filepath.ToSlash(rel)
				if !includedByGlob(rel, include) || excludedByGlob(rel, exclude) {
					return nil
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				for index, line := range strings.Split(string(data), "\n") {
					if len(extractor.extract(line)) > 0 {
						target.WriteString(rgMatchFixture(displayCharterPath(plan.DisplayRoot, rel), index+1, line+"\n"))
					}
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
			fileInventory := ""
			if plan.ReferenceFile != "" {
				data, err := os.ReadFile(plan.ReferenceFile)
				if err != nil {
					return nil, err
				}
				fileInventory = string(data)
			}
			dirInventory, err := testDirectoryInventory(plan.ReferenceDir)
			if err != nil {
				return nil, err
			}
			output := refTargetMarker + "\n" + target.String() +
				refFileMarker + "\n" + fileInventory +
				refDirMarker + "\n" + dirInventory
			reduced, err := ReduceRefSearch(plan, output)
			if err != nil {
				return nil, err
			}
			findings = append(findings, reduced...)
		}
	}
	SortFindings(findings)
	return findings, nil
}

func testDirectoryInventory(root string) (string, error) {
	if root == "" {
		return "", nil
	}
	var values []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		values = append(values, filepath.ToSlash(rel))
		return nil
	})
	return strings.Join(values, "\n"), err
}

func refCharter(id string, check CharterCheck) Charter {
	return Charter{ID: id, Checks: []CharterCheck{check}}
}
