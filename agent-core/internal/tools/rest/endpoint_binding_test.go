// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestValidateEndpointRejectsUnknownBinding proves an endpoint whose binding the
// runtime does not handle (handleEndpoint would return 501) is rejected at
// config-validation time, so --validate-config cannot approve it (#510).
func TestValidateEndpointRejectsUnknownBinding(t *testing.T) {
	t.Parallel()
	err := validateEndpoint("e", Endpoint{Method: "GET", Path: "/x", Binding: "totally_bogus"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown binding")
	require.Contains(t, err.Error(), "totally_bogus")
	// The diagnostic lists the accepted bindings.
	require.Contains(t, err.Error(), bindingHealth)
}

// TestValidateEndpointRejectsEmptyBinding proves a missing binding is rejected
// with a path-specific diagnostic rather than defaulting into the 501 handler.
func TestValidateEndpointRejectsEmptyBinding(t *testing.T) {
	t.Parallel()
	err := validateEndpoint("e", Endpoint{Method: "GET", Path: "/x"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no binding")
}

// TestValidateEndpointAcceptsEveryHandledBinding proves every binding in the
// closed set clears the binding-membership check (it may still fail on its own
// required sub-config, but never with the unknown/empty-binding error), so the
// enum stays consistent with what handleEndpoint dispatches.
func TestValidateEndpointAcceptsEveryHandledBinding(t *testing.T) {
	t.Parallel()
	for binding := range handledServerBindings {
		err := validateEndpoint("e", Endpoint{Method: "GET", Path: "/x", Binding: binding})
		if err != nil {
			require.NotContains(t, err.Error(), "unknown binding", "binding %q rejected as unknown", binding)
			require.NotContains(t, err.Error(), "no binding", "binding %q rejected as empty", binding)
		}
	}
}

// TestValidateDefinitionRejectsUnknownBinding proves the rejection reaches the
// public ValidateDefinition entry point used by --validate-config.
func TestValidateDefinitionRejectsUnknownBinding(t *testing.T) {
	t.Parallel()
	err := ValidateDefinition(singleServerDefinition(Endpoint{Method: "GET", Path: "/x", Binding: "nope"}))
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "unknown binding"), "got: %v", err)
}
