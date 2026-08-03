// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// A body_source none operation discards the prior Result's output, so an input
// carrying an authority-named field (here method, from a preceding HTTP control
// event) must not fail the runtime-authority guard; params end up empty.
func TestNormalizeRuntimeParams_BodySourceNoneIgnoresPriorAuthorityFields(t *testing.T) {
	t.Parallel()

	input := map[string]interface{}{
		"method": "POST",
		"url":    "http://evil",
		"path":   "/api/lifecycle/exit",
	}
	params, err := normalizeRuntimeParams(input, RequestBinding{BodySource: bodySourceNone}, nil)
	require.NoError(t, err)
	require.Empty(t, params)
}

// The passthrough body source still becomes the params directly, so a runtime
// input naming transport authority is rejected as before.
func TestNormalizeRuntimeParams_PassthroughRejectsAuthority(t *testing.T) {
	t.Parallel()

	for _, field := range []string{"method", "url", "auth_ref", "base_url"} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			_, err := normalizeRuntimeParams(map[string]interface{}{field: "x"}, RequestBinding{}, nil)
			require.ErrorContains(t, err, "cannot set REST authority")
		})
	}
}

// The passthrough body source keeps accepting declared params.
func TestNormalizeRuntimeParams_PassthroughAcceptsDeclaredParams(t *testing.T) {
	t.Parallel()

	binding := RequestBinding{Query: map[string]interface{}{"owner": nil}}
	params, err := normalizeRuntimeParams(map[string]interface{}{"owner": "nokia"}, binding, nil)
	require.NoError(t, err)
	require.Equal(t, "nokia", params["owner"])
}
