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
