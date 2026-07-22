// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"strings"
	"testing"
)

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

func TestPackageProductionFilesStaySplit(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		require.NotEqual(t, "types.go", entry.Name())
		data, err := os.ReadFile(entry.Name())
		require.NoError(t, err)
		lines := strings.Count(string(data), "\n")
		require.LessOrEqual(t, lines, 500, "%s exceeds production file limit", entry.Name())
	}
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

// TestValidate_ObjectTouchpointNotOrphaned proves the GH-448 fix end to end: a use
// case whose touchpoint uses the object form ({id, target, reason}) -- as the
// example corpora author them -- builds a touches edge and an acceptance-criterion
// citation, so the SRD is neither orphaned nor flagged as a bare touchpoint.
func TestValidate_ObjectTouchpointNotOrphaned(t *testing.T) {
	corpus := &Corpus{
		SRDs: map[string]SRD{
			"srd004-coordinator": {
				ID:                 "srd004-coordinator",
				AcceptanceCriteria: []AcceptanceCriterion{{ID: "AC1", Criterion: "The coordinator binds the intent."}},
			},
		},
		UseCases: map[string]UseCase{
			"rel05.0-uc001": {
				ID: "rel05.0-uc001",
				// The parser has already folded the {id, target, reason} object into
				// this canonical string (see TestParseUseCase_ObjectTouchpoints).
				Touchpoints: []string{"srd004-coordinator AC1 -- The coordinator binds the intent."},
			},
		},
		SRDOrder: []string{"srd004-coordinator"},
		UCOrder:  []string{"rel05.0-uc001"},
	}

	g, err := BuildGraph(corpus)
	require.NoError(t, err)

	assert.Contains(t, g.OutgoingByRel("rel05.0-uc001", RelTouches), "srd004-coordinator",
		"the object-form touchpoint must build a touches edge")
	assert.Contains(t, g.OutgoingByRel("rel05.0-uc001", RelCites), "srd004-coordinator:AC1",
		"the cited acceptance criterion must build a cites edge")
	assert.Empty(t, checkOrphanedSRDs(g), "the touched SRD must not be orphaned")
	assert.Empty(t, checkBareTouchpoints(g, corpus), "an AC-citing touchpoint is not bare")
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

	for _, f := range findings {
		assert.NotContains(t, f.Message, "srd001-auth:AC1",
			"AC1 and AC2 are covered by test cases")
	}
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
		assert.Contains(t, f.Message, "00.1", "the fixture keeps 00.1 without a test suite; 00.0 has test-rel00.0")
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

func TestValidate_FormatFindingsWithProvenance(t *testing.T) {
	findings := []Finding{
		{
			Level:   "warning",
			SuiteID: "paper-charter",
			CheckID: "citations-resolve",
			Kind:    "ref_check",
			File:    "paper/main.md",
			Line:    12,
			Message: "citation @missing does not resolve",
		},
		{
			Level:   "error",
			SuiteID: "paper-charter",
			CheckID: "no-internal-vocabulary",
			Kind:    "grep_check",
			File:    "paper/main.md",
			Line:    4,
			Message: "found forbidden term cobbler",
		},
	}

	output := FormatFindings(findings)

	assert.Contains(t, output, "[error] paper-charter/no-internal-vocabulary (grep_check):")
	assert.Contains(t, output, "  - paper/main.md:4: found forbidden term cobbler")
	assert.Contains(t, output, "[warning] paper-charter/citations-resolve (ref_check):")
	assert.Contains(t, output, "  - paper/main.md:12: citation @missing does not resolve")
	assert.Less(t, strings.Index(output, "[error]"), strings.Index(output, "[warning]"))
}

func TestValidate_FormatFindingsSortsDeterministicallyWithoutMutatingInput(t *testing.T) {
	findings := []Finding{
		{Level: "warning", SuiteID: "suite-b", CheckID: "b", File: "z.md", Line: 2, Message: "second"},
		{Level: "warning", SuiteID: "suite-a", CheckID: "a", File: "a.md", Line: 1, Message: "first"},
		{Level: "error", Check: "legacy-error", Message: "legacy"},
	}
	original := append([]Finding(nil), findings...)

	output := FormatFindings(findings)

	assert.Equal(t, original, findings, "FormatFindings must not reorder caller-owned slices")
	assert.Less(t, strings.Index(output, "[error] legacy-error"), strings.Index(output, "[warning] suite-a/a"))
	assert.Less(t, strings.Index(output, "[warning] suite-a/a"), strings.Index(output, "[warning] suite-b/b"))
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

func TestValidate_ActionResolvesEdges(t *testing.T) {
	g, _ := loadTestGraphAndCorpus(t)

	resolvesEdges := g.EdgesByRel(RelResolves)
	assert.NotEmpty(t, resolvesEdges, "transition actions should resolve to tool declarations")
}
