// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadTestGraphAndCorpus(t *testing.T) (*Graph, *Corpus) {
	t.Helper()
	c, err := LoadCorpus(filepath.Join("testdata", "valid"))
	require.NoError(t, err)
	g, err := BuildGraph(c)
	require.NoError(t, err)
	return g, c
}

func TestValidate_ReturnsFindings(t *testing.T) {
	g, c := loadTestGraphAndCorpus(t)
	findings := Validate(g, c)

	assert.NotEmpty(t, findings, "fixture corpus has known issues (orphaned SRD, uncovered items)")

	byCheck := make(map[string]int)
	for _, f := range findings {
		byCheck[f.Check]++
	}

	assert.Greater(t, byCheck["orphaned-srd"], 0, "srd003-storage has no UC touchpoint")
	assert.Greater(t, byCheck["uncovered-req-item"], 0, "some items lack AC coverage")
}

func TestValidate_OrphanedSRD(t *testing.T) {
	g, c := loadTestGraphAndCorpus(t)

	findings := checkOrphanedSRDs(g)

	orphanedIDs := make(map[string]bool)
	for _, f := range findings {
		if f.Check == "orphaned-srd" {
			for _, srd := range g.NodesByKind(KindSRD) {
				if contains(f.Message, srd.ID) {
					orphanedIDs[srd.ID] = true
				}
			}
		}
	}

	assert.False(t, orphanedIDs["srd001-auth"], "srd001-auth is referenced by use case")
	assert.False(t, orphanedIDs["srd002-api"], "srd002-api is referenced by use case")
	assert.True(t, orphanedIDs["srd003-storage"], "srd003-storage has no use case touchpoint")

	_ = c
}

func TestValidate_UncoveredReqItems(t *testing.T) {
	g, _ := loadTestGraphAndCorpus(t)

	findings := checkUncoveredReqItems(g)

	var uncovered []string
	for _, f := range findings {
		uncovered = append(uncovered, f.Message)
	}

	assert.NotEmpty(t, uncovered, "some req items should be uncovered (srd002, srd003 ACs don't trace all items)")
}

func TestValidate_UncoveredACs(t *testing.T) {
	g, _ := loadTestGraphAndCorpus(t)

	findings := checkUncoveredACs(g)

	var uncoveredIDs []string
	for _, f := range findings {
		uncoveredIDs = append(uncoveredIDs, f.Message)
	}

	for _, f := range findings {
		assert.NotContains(t, f.Message, "srd001-auth:AC1",
			"AC1 and AC2 are covered by test cases")
	}
}

func TestValidate_TestSuiteCoversUseCase(t *testing.T) {
	g, _ := loadTestGraphAndCorpus(t)

	findings := checkOrphanedTestSuites(g)
	assert.Empty(t, findings, "test-rel00.0 traces rel00.0-uc001-login which exists")
}

func TestValidate_BareTouchpoints(t *testing.T) {
	g, c := loadTestGraphAndCorpus(t)

	findings := checkBareTouchpoints(g, c)
	assert.Empty(t, findings, "all touchpoints in fixture specify R-groups")
}

func TestValidate_ReleasesWithoutTestSuites(t *testing.T) {
	g, c := loadTestGraphAndCorpus(t)

	findings := checkReleasesWithoutTestSuites(g, c)
	for _, f := range findings {
		assert.Contains(t, f.Message, "00.1", "release 00.1 has no test suite in fixture; 00.0 has test-rel00.0")
	}
}

func TestValidate_FormatFindings(t *testing.T) {
	findings := []Finding{
		{Check: "orphaned-srd", Level: "warning", Message: "SRD srd003-storage not referenced"},
		{Check: "broken-citation", Level: "error", Message: "use case uc1 cites missing group"},
	}

	output := FormatFindings(findings)
	assert.Contains(t, output, "[error] broken-citation")
	assert.Contains(t, output, "[warning] orphaned-srd")
}

func TestValidate_FormatEmpty(t *testing.T) {
	output := FormatFindings(nil)
	assert.Contains(t, output, "All consistency checks passed")
}

func TestValidate_ErrorsAndWarnings(t *testing.T) {
	findings := []Finding{
		{Check: "a", Level: "error", Message: "e1"},
		{Check: "b", Level: "warning", Message: "w1"},
		{Check: "c", Level: "error", Message: "e2"},
	}
	assert.Len(t, Errors(findings), 2)
	assert.Len(t, Warnings(findings), 1)
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
