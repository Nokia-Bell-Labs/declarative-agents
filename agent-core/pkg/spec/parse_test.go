// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSRD(t *testing.T) {
	srd, err := ParseSRD(filepath.Join("testdata", "valid", "docs", "specs",
		"software-requirements", "srd001-auth.yaml"))
	require.NoError(t, err)

	assert.Equal(t, "srd001-auth", srd.ID)
	assert.Equal(t, "Authentication", srd.Title)
	assert.Equal(t, []string{"G1: Secure authentication."}, srd.Goals)
	assert.Equal(t, []string{"R1", "R2"}, srd.OrderedGroups)

	r1 := srd.Requirements["R1"]
	assert.Equal(t, "Login", r1.Title)
	require.Len(t, r1.Items, 3)
	assert.Equal(t, "R1.1", r1.Items[0].ID)
	assert.Equal(t, 1, r1.Items[0].Weight)
	assert.Equal(t, 2, r1.Items[1].Weight)

	items := srd.AllItems()
	assert.Len(t, items, 4)

	require.Len(t, srd.AcceptanceCriteria, 2)
	assert.Equal(t, "AC1", srd.AcceptanceCriteria[0].ID)
	assert.Equal(t, []string{"R1.1", "R1.2", "R1.3"}, srd.AcceptanceCriteria[0].Traces)
}

func TestParseSRDRejectsMalformedRequirementShapes(t *testing.T) {
	t.Parallel()
	const source = "docs/specs/software-requirements/srd-malformed.yaml"
	tests := []struct {
		name string
		yaml string
		want []string
	}{
		{
			name: "scalar items",
			yaml: "requirements:\n  R1:\n    items: R1.1\n",
			want: []string{"group R1", "items: expected sequence"},
		},
		{
			name: "mapping items",
			yaml: "requirements:\n  R1:\n    items:\n      R1.1: text\n",
			want: []string{"group R1", "items: expected sequence"},
		},
		{
			name: "null items",
			yaml: "requirements:\n  R1:\n    items: null\n",
			want: []string{"group R1", "items: expected sequence"},
		},
		{
			name: "mixed sequence items",
			yaml: "requirements:\n  R1:\n    items:\n      - R1.1: valid\n      - malformed\n",
			want: []string{"group R1", "items: expected mapping entry"},
		},
		{
			name: "requirements nested as sequence",
			yaml: "requirements:\n  - R1:\n      items:\n        - R1.1: valid\n",
			want: []string{"requirements: expected mapping"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseSRDBytes([]byte("id: srd-malformed\n"+tt.yaml), source)

			require.Error(t, err)
			assert.ErrorContains(t, err, source)
			assert.ErrorContains(t, err, "requirements")
			for _, fragment := range tt.want {
				assert.ErrorContains(t, err, fragment)
			}
		})
	}
}

func TestParseSRD_NonGoals(t *testing.T) {
	srd, err := ParseSRD(filepath.Join("testdata", "valid", "docs", "specs",
		"software-requirements", "srd003-storage.yaml"))
	require.NoError(t, err)

	assert.Equal(t, []string{"N1: NoSQL support.", "N2: Distributed transactions."}, srd.NonGoals)
	assert.Equal(t, 3, srd.Requirements["R1"].Items[0].Weight)
}

func TestParseSRD_DependsOn(t *testing.T) {
	srd, err := ParseSRD(filepath.Join("testdata", "valid", "docs", "specs",
		"software-requirements", "srd002-api.yaml"))
	require.NoError(t, err)

	require.Len(t, srd.DependsOn, 1)
	assert.Equal(t, "srd001-auth", srd.DependsOn[0].SRDID)
}

func TestParseUseCase(t *testing.T) {
	uc, err := ParseUseCase(filepath.Join("testdata", "valid", "docs", "specs",
		"use-cases", "rel00.0-uc001-login.yaml"))
	require.NoError(t, err)

	assert.Equal(t, "rel00.0-uc001-login", uc.ID)
	assert.Equal(t, "Login flow", uc.Title)
	assert.Len(t, uc.Flow, 3)
	assert.Len(t, uc.Touchpoints, 2)
	assert.Len(t, uc.SuccessCriteria, 2)
	assert.Equal(t, "test-rel00.0", uc.TestSuite)
}

func TestParseUseCase_TaggedTouchpoints(t *testing.T) {
	path := writeUseCaseFile(t, `id: rel00.0-uc999-tagged
title: Tagged touchpoint
summary: Tagged touchpoint parsing.
actor: Tester
trigger: Parse use case.
flow:
  - F1: Parse the file.
touchpoints:
  - T1: "srd001-auth R1, R2 -- authentication flow"
success_criteria: []
out_of_scope: []
test_suite: test-rel00.0
`)

	uc, err := ParseUseCase(path)
	require.NoError(t, err)

	assert.Equal(t, []string{"T1: srd001-auth R1, R2 -- authentication flow"}, uc.Touchpoints)
}

func TestParseUseCase_ObjectTouchpoints(t *testing.T) {
	path := writeUseCaseFile(t, `id: rel05.0-uc001-object
title: Object touchpoint
summary: Object touchpoint parsing.
actor: Tester
trigger: Parse use case.
flow:
  - F1: Parse the file.
touchpoints:
  - id: T1
    target: srd004-coordinator AC1
    reason: The coordinator binds the provisioning intent.
  - id: T2
    target: srd005-creator AC2
    reason: The creator realizes the instance.
success_criteria: []
out_of_scope: []
test_suite: test-rel05.0
`)

	uc, err := ParseUseCase(path)
	require.NoError(t, err)

	// Each {id, target, reason} object folds into one canonical string; the id
	// label is dropped and the reason follows the "--" separator.
	assert.Equal(t, []string{
		"srd004-coordinator AC1 -- The coordinator binds the provisioning intent.",
		"srd005-creator AC2 -- The creator realizes the instance.",
	}, uc.Touchpoints)

	// The folded string parses back into an SRD touches edge and an AC citation.
	srdID, groups := parseTouchpoint(uc.Touchpoints[0])
	assert.Equal(t, "srd004-coordinator", srdID)
	assert.Equal(t, []string{"AC1"}, groups)
}

func TestParseTestSuite(t *testing.T) {
	ts, err := ParseTestSuite(filepath.Join("testdata", "valid", "docs", "specs",
		"test-suites", "test-rel00.0.yaml"))
	require.NoError(t, err)

	assert.Equal(t, "test-rel00.0", ts.ID)
	assert.Equal(t, "00.0", ts.Release)
	assert.Len(t, ts.TestCases, 2)
	assert.Equal(t, "TestLogin_ValidCredentials", ts.TestCases[0].Name)
	assert.Equal(t, []string{"srd001-auth AC1"}, ts.TestCases[0].Traces)
}

func writeUseCaseFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "use-case.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestParseSRD_FileNotFound(t *testing.T) {
	_, err := ParseSRD("nonexistent.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read SRD")
}

func TestParseRoadmap(t *testing.T) {
	rm, err := ParseRoadmap(filepath.Join("testdata", "valid", "docs", "road-map.yaml"))
	require.NoError(t, err)

	assert.Len(t, rm.Releases, 2)
	assert.Equal(t, []string{"00.0", "00.1"}, rm.ReleaseVersions())
}

func TestParseSpecIndex(t *testing.T) {
	si, err := ParseSpecIndex(filepath.Join("testdata", "valid", "docs", "SPECIFICATIONS.yaml"))
	require.NoError(t, err)

	assert.Equal(t, []string{"srd001-auth", "srd002-api", "srd003-storage"}, si.SRDIDs())
	assert.Len(t, si.UseCaseIndex, 1)
	assert.Len(t, si.TestSuiteIndex, 1)
}
