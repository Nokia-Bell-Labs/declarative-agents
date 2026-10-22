// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package spec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// architectureWith returns a minimal canonical ARCHITECTURE.md whose
// related_documents field holds body.
func architectureWith(body string) string {
	return "---\nid: arch-test\ntitle: Test Architecture\n---\n\n" +
		"<!-- field: overview -->\n## Overview\n\nOne component.\n\n" +
		"<!-- field: interfaces -->\n## Interfaces\n\n" +
		"<!-- field: components -->\n## Components\n\n" +
		"<!-- field: design_decisions -->\n## Design decisions\n\n" +
		"<!-- field: technology_choices -->\n## Technology choices\n\n" +
		"<!-- field: project_structure -->\n## Project structure\n\nsrc/\n\n" +
		"<!-- field: implementation_status -->\n## Implementation status\n\n" +
		"<!-- field: current_focus -->\n### Current focus\n\nDone.\n\n" +
		"<!-- field: related_documents -->\n## Related documents\n\n" + body
}

// loadWithArchitecture loads the valid fixture corpus with an
// ARCHITECTURE.md whose related_documents field holds body.
func loadWithArchitecture(t *testing.T, body string) *Corpus {
	t.Helper()
	root := copyTree(t, "testdata/valid")
	path := filepath.Join(root, ArchFile)
	require.NoError(t, os.WriteFile(path, []byte(architectureWith(body)), 0o600))
	c, err := LoadCorpus(root)
	require.NoError(t, err)
	for _, f := range c.FormatFindings {
		require.NotEqual(t, ArchFile, f.File, "fixture architecture must be canonical: %s", f.Message)
	}
	return c
}

func TestRelatedDocuments_PlainListLoads(t *testing.T) {
	c := loadWithArchitecture(t, "- docs/road-map.md\n- docs/MISSING.md\n")

	assert.Equal(t, []string{"docs/road-map.md", "docs/MISSING.md"}, c.RelatedDocuments)
	assert.Empty(t, c.MalformedRelatedDocuments)

	findings := checkSpecIndexPaths(c)
	require.Len(t, findings, 1)
	assert.Equal(t, "index-broken-path", findings[0].Check)
	assert.Contains(t, findings[0].Message, `related_documents entry "docs/MISSING.md"`)
}

func TestRelatedDocuments_TableRowsLoad(t *testing.T) {
	c := loadWithArchitecture(t, "| doc | purpose |\n| --- | --- |\n"+
		"| docs/road-map.md | Release plan. |\n| docs/MISSING.md | Gone. |\n")

	assert.Equal(t, []string{"docs/road-map.md", "docs/MISSING.md"}, c.RelatedDocuments)
	assert.Empty(t, c.MalformedRelatedDocuments)
	require.Len(t, checkSpecIndexPaths(c), 1)
}

func TestRelatedDocuments_MalformedEntryIsAFinding(t *testing.T) {
	c := loadWithArchitecture(t, "| path | purpose |\n| --- | --- |\n| docs/road-map.md | Release plan. |\n")

	assert.Empty(t, c.RelatedDocuments)
	require.Len(t, c.MalformedRelatedDocuments, 1)

	findings := checkMalformedRelatedDocuments(c)
	require.Len(t, findings, 1)
	assert.Equal(t, "related-document-malformed", findings[0].Check)
	assert.Equal(t, "error", findings[0].Level)
	assert.Equal(t, ArchFile, findings[0].File)
	assert.Contains(t, findings[0].Message, "entry 1 has no doc")

	g, err := BuildGraph(c)
	require.NoError(t, err)
	var checks []string
	for _, f := range Validate(g, c) {
		checks = append(checks, f.Check)
	}
	assert.Contains(t, checks, "related-document-malformed", "the other checks run alongside it")
}

func TestRelatedDocuments_NoArchitecture(t *testing.T) {
	_, c := loadTestGraphAndCorpus(t)
	assert.Empty(t, c.RelatedDocuments)
	assert.Empty(t, c.MalformedRelatedDocuments)
}

func TestValidate_SpecIndexPaths_EveryTable(t *testing.T) {
	missing := "docs/missing.md"
	corpus := &Corpus{
		RootDir: t.TempDir(),
		SpecIndex: SpecIndex{
			FoundationDocumentIndex: []DocumentEntry{{ID: "vision", Path: missing}},
			SRDIndex:                []SRDEntry{{ID: "srd001", Path: missing}},
			ConfigFormatIndex:       []DocumentEntry{{ID: "cfg-x", Path: missing}},
			SemanticModelIndex:      []DocumentEntry{{ID: "sm-x", Path: missing}},
			UseCaseIndex:            []UseCaseEntry{{ID: "rel00.0-uc001", Path: missing}},
			TestSuiteIndex:          []TestSuiteEntry{{ID: "test-rel00.0", Path: missing}},
		},
		RelatedDocuments: []string{missing},
	}

	findings := checkSpecIndexPaths(corpus)
	require.Len(t, findings, 7)
	for i, table := range []string{
		"foundation_document_index entry vision",
		"srd_index entry srd001",
		"config_format_index entry cfg-x",
		"semantic_model_index entry sm-x",
		"use_case_index entry rel00.0-uc001",
		"test_suite_index entry test-rel00.0",
		"related_documents entry",
	} {
		assert.Equal(t, "index-broken-path", findings[i].Check)
		assert.Contains(t, findings[i].Message, table)
	}
}

func TestValidate_SpecIndexPaths_RootRelativeOnly(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs", "specs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "specs", "sm.md"), nil, 0o600))
	corpus := &Corpus{
		RootDir: root,
		SpecIndex: SpecIndex{SemanticModelIndex: []DocumentEntry{
			{ID: "sm-root", Path: "docs/specs/sm.md"},
			{ID: "sm-doc-relative", Path: "specs/sm.md"},
		}},
	}

	findings := checkSpecIndexPaths(corpus)
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0].Message, "sm-doc-relative")
}

func TestRelatedDocumentPaths_Shapes(t *testing.T) {
	for _, tc := range []struct {
		name      string
		yaml      string
		paths     []string
		malformed int
	}{
		{"absent", "other: 1\n", nil, 0},
		{"plain list", "related_documents: [a.md, b.md]\n", []string{"a.md", "b.md"}, 0},
		{"rows", "related_documents:\n  - {doc: a.md, purpose: x}\n", []string{"a.md"}, 0},
		{"row without doc", "related_documents:\n  - {path: a.md}\n", nil, 1},
		{"nested list", "related_documents:\n  - [a.md]\n  - b.md\n", []string{"b.md"}, 1},
		{"not a list", "related_documents: a.md\n", nil, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var doc struct {
				RelatedDocuments yaml.Node `yaml:"related_documents"`
			}
			require.NoError(t, yaml.Unmarshal([]byte(tc.yaml), &doc))
			paths, malformed := relatedDocumentPaths(&doc.RelatedDocuments)
			assert.Equal(t, tc.paths, paths)
			assert.Len(t, malformed, tc.malformed)
		})
	}
}
