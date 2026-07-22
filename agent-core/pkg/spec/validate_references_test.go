// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestValidate_TestSuiteCoversUseCase(t *testing.T) {
	g, _ := loadTestGraphAndCorpus(t)

	findings := checkOrphanedTestSuites(g)
	assert.Empty(t, findings, "test-rel00.0 traces rel00.0-uc001-login which exists")
}

func TestValidate_UseCaseIndexRefs(t *testing.T) {
	corpus := &Corpus{
		SpecIndex: SpecIndex{
			UseCaseIndex: []UseCaseEntry{
				{ID: "exists"},
				{ID: "missing"},
			},
		},
		UseCases: map[string]UseCase{"exists": {ID: "exists"}},
	}
	findings := checkUseCaseIndexRefs(corpus)
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0].Message, "missing")
	assert.Equal(t, "error", findings[0].Level)
}

func TestValidate_TestSuiteIndexRefs(t *testing.T) {
	corpus := &Corpus{
		SpecIndex: SpecIndex{
			TestSuiteIndex: []TestSuiteEntry{
				{ID: "exists"},
				{ID: "missing"},
			},
		},
		TestSuites: map[string]TestSuite{"exists": {ID: "exists"}},
	}
	findings := checkTestSuiteIndexRefs(corpus)
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0].Message, "missing")
}

func TestValidate_RoadmapUseCaseRefs(t *testing.T) {
	corpus := &Corpus{
		Roadmap: Roadmap{
			Releases: []Release{
				{Version: "1.0", UseCases: []UseCaseRef{{ID: "exists"}, {ID: "missing"}}},
			},
		},
		UseCases: map[string]UseCase{"exists": {ID: "exists"}},
	}
	findings := checkRoadmapUseCaseRefs(corpus)
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0].Message, "missing")
}

func TestValidate_UseCaseTestSuiteReciprocity(t *testing.T) {
	corpus := &Corpus{
		UseCases: map[string]UseCase{
			"uc1": {ID: "uc1", TestSuite: "ts-good"},
			"uc2": {ID: "uc2", TestSuite: "ts-no-trace"},
			"uc3": {ID: "uc3", TestSuite: "ts-missing"},
		},
		TestSuites: map[string]TestSuite{
			"ts-good":     {ID: "ts-good", Traces: []string{"uc1"}},
			"ts-no-trace": {ID: "ts-no-trace", Traces: []string{"other"}},
		},
		UCOrder: []string{"uc1", "uc2", "uc3"},
	}
	findings := checkUseCaseTestSuiteReciprocity(corpus)
	require.Len(t, findings, 2)

	var checks []string
	for _, f := range findings {
		checks = append(checks, f.Check)
	}
	assert.Contains(t, checks, "use-case-missing-test-suite")
	assert.Contains(t, checks, "test-suite-missing-uc-trace")
}

func TestValidate_SpecIndexPaths(t *testing.T) {
	g, c := loadTestGraphAndCorpus(t)
	_ = g

	findings := checkSpecIndexPaths(c)
	for _, f := range findings {
		t.Logf("finding: %s", f.Message)
	}
	assert.Empty(t, findings, "all fixture paths should exist")
}

func TestValidate_SpecIndexPaths_Broken(t *testing.T) {
	corpus := &Corpus{
		RootDir: t.TempDir(),
		SpecIndex: SpecIndex{
			SRDIndex: []SRDEntry{
				{ID: "srd-bad", Path: "docs/nonexistent.yaml"},
			},
		},
	}
	findings := checkSpecIndexPaths(corpus)
	require.Len(t, findings, 1)
	assert.Equal(t, "index-broken-path", findings[0].Check)
}

func TestValidate_FixtureIndexConsistency(t *testing.T) {
	g, c := loadTestGraphAndCorpus(t)

	ucFindings := checkUseCaseIndexRefs(c)
	assert.Empty(t, ucFindings, "fixture UC index should be consistent")

	tsFindings := checkTestSuiteIndexRefs(c)
	assert.Empty(t, tsFindings, "fixture TS index should be consistent")

	_ = g
}

func TestValidate_DocSpecsLoaded(t *testing.T) {
	_, c := loadTestGraphAndCorpus(t)

	require.Contains(t, c.DocSpecs, "sm-test-model")
	require.Contains(t, c.DocSpecs, "cfg-test-format")

	sm := c.DocSpecs["sm-test-model"]
	assert.Equal(t, "Test Semantic Model", sm.Title)
	assert.Len(t, sm.RequirementsSource.Canonical, 1)
	assert.Contains(t, sm.RelatedDocuments, "cfg-test-format")

	cf := c.DocSpecs["cfg-test-format"]
	assert.Len(t, cf.Implementation.Paths, 1)
	assert.Len(t, cf.Examples, 1)
}

func TestValidate_DocSpecRequirementsSources(t *testing.T) {
	corpus := &Corpus{
		RootDir: t.TempDir(),
		DocSpecs: map[string]DocSpec{
			"bad": {
				ID: "bad",
				RequirementsSource: DocSpecSources{
					Canonical: []string{"docs/nonexistent.yaml"},
				},
			},
		},
	}
	findings := checkDocSpecRequirementsSources(corpus)
	require.Len(t, findings, 1)
	assert.Equal(t, "docspec-broken-requirement-source", findings[0].Check)
}

func TestValidate_DocSpecRequirementsSources_Fixture(t *testing.T) {
	_, c := loadTestGraphAndCorpus(t)
	findings := checkDocSpecRequirementsSources(c)
	assert.Empty(t, findings, "fixture canonical sources should exist")
}

func TestValidate_DocSpecRelatedDocuments(t *testing.T) {
	corpus := &Corpus{
		SRDs: map[string]SRD{"srd001-auth": {ID: "srd001-auth"}},
		DocSpecs: map[string]DocSpec{
			"spec-a": {
				ID:               "spec-a",
				RelatedDocuments: []string{"srd001-auth", "unknown-ref"},
			},
		},
	}
	findings := checkDocSpecRelatedDocuments(corpus)
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0].Message, "unknown-ref")
}

func TestValidate_DocSpecRelatedDocuments_Fixture(t *testing.T) {
	_, c := loadTestGraphAndCorpus(t)
	findings := checkDocSpecRelatedDocuments(c)
	assert.Empty(t, findings, "fixture related documents should all resolve")
}

func TestValidate_DocSpecImplementationPaths(t *testing.T) {
	corpus := &Corpus{
		RootDir: t.TempDir(),
		DocSpecs: map[string]DocSpec{
			"bad": {
				ID:             "bad",
				Implementation: DocSpecImpl{Paths: []string{"pkg/nonexistent.go"}},
			},
		},
	}
	findings := checkDocSpecImplementationPaths(corpus)
	require.Len(t, findings, 1)
	assert.Equal(t, "docspec-broken-implementation-path", findings[0].Check)
}

func TestValidate_DocSpecImplementationPaths_Fixture(t *testing.T) {
	_, c := loadTestGraphAndCorpus(t)
	findings := checkDocSpecImplementationPaths(c)
	assert.Empty(t, findings, "fixture implementation paths should exist")
}

func TestValidate_DocSpecExamplePaths(t *testing.T) {
	corpus := &Corpus{
		RootDir: t.TempDir(),
		DocSpecs: map[string]DocSpec{
			"bad": {
				ID:       "bad",
				Examples: []DocSpecExample{{File: "nonexistent/file.yaml"}},
			},
		},
	}
	findings := checkDocSpecExamplePaths(corpus)
	require.Len(t, findings, 1)
	assert.Equal(t, "docspec-broken-example-path", findings[0].Check)
}

func TestValidate_DocSpecExamplePaths_Fixture(t *testing.T) {
	_, c := loadTestGraphAndCorpus(t)
	findings := checkDocSpecExamplePaths(c)
	assert.Empty(t, findings, "fixture example paths should exist")
}
