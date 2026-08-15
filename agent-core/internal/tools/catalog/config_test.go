// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateParseStructuredConfigAcceptsAdjacentOutput(t *testing.T) {
	base := ParseStructuredConfig{
		Schema: map[string]interface{}{"type": "object"},
		Parsed: "Parsed", Unparsed: "Unparsed",
	}

	for _, source := range []string{"$.output", "$from(model).response"} {
		cfg := base
		cfg.Source = source
		require.NoError(t, ValidateParseStructuredConfig("validate_response", cfg))
	}
}

func TestValidateParseStructuredConfigRejectsOtherCurrentResultPaths(t *testing.T) {
	cfg := ParseStructuredConfig{
		Source: "$.response", Schema: map[string]interface{}{"type": "object"},
		Parsed: "Parsed", Unparsed: "Unparsed",
	}

	err := ValidateParseStructuredConfig("validate_response", cfg)

	require.ErrorContains(t, err, "must be $.output or a $from(label).path selector")
}
