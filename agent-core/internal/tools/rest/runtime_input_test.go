// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateRuntimeInputRejectsAuthorityOverride(t *testing.T) {
	t.Parallel()

	for _, field := range []string{"method", "url", "auth_ref", "redirect_policy", "host"} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			err := ValidateRuntimeInput(map[string]interface{}{field: "malicious"})
			require.ErrorContains(t, err, "cannot set REST authority")
		})
	}
}

func TestValidateRuntimeInputAcceptsDeclaredValues(t *testing.T) {
	t.Parallel()

	err := ValidateRuntimeInput(map[string]interface{}{
		"owner": "nokia",
		"repo":  "agent-core",
		"body": map[string]interface{}{
			"title": "issue",
		},
	})
	require.NoError(t, err)
}

func TestRuntimeParamsFromNonJSONSeed(t *testing.T) {
	t.Parallel()

	// The machine seed output ("Begin.") and any non-REST word's plain-text
	// output are not JSON objects. A REST client word may still be the first
	// action in a sentence, so these carry no runtime parameters rather than
	// failing the build.
	for _, output := range []string{"Begin.", "Resume.", "plain text"} {
		params, err := runtimeParams(output)
		require.NoError(t, err)
		require.Empty(t, params)
	}
}

func TestRuntimeParamsFromJSONEnvelope(t *testing.T) {
	t.Parallel()

	params, err := runtimeParams(`{"parameters":{"path":"a.md"}}`)
	require.NoError(t, err)
	require.Equal(t, "a.md", params["path"])
}
