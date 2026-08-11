// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBrokenTouchpointCheckReadsAuthoredCorpusReference(t *testing.T) {
	t.Parallel()
	corpus := &Corpus{
		SRDs: map[string]SRD{"srd001-existing": {ID: "srd001-existing"}},
		UseCases: map[string]UseCase{"uc-missing": {
			ID: "uc-missing", Touchpoints: []string{"srd999-missing R1"},
		}},
		UCOrder: []string{"uc-missing"},
	}
	findings := checkBrokenTouchpoints(corpus)
	require.Len(t, findings, 1)
	require.Equal(t, "broken-touchpoint", findings[0].Check)
	require.Equal(t, "error", findings[0].Level)
	require.Contains(t, findings[0].Message, "srd999-missing")
}

func TestDependsOnViolationIsNotASelectableDuplicateOfLoadValidation(t *testing.T) {
	t.Parallel()
	require.False(t, supportedSpecCorpusCheckIDs["depends-on-violation"])
	charter := Charter{
		ID: "removed-check", Checks: []CharterCheck{{
			ID: "depends", Kind: "spec_corpus", Checks: []string{"depends-on-violation"},
		}},
	}
	err := validateCharter(&charter)
	require.ErrorContains(t, err, `unknown spec_corpus check "depends-on-violation"`)
}
