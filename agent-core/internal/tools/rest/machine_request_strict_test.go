// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const validMachineRequestDoc = `
rest:
  version: v1
  servers:
    srv:
      address: 127.0.0.1:0
      endpoints:
        e:
          method: POST
          path: /doc
          binding: machine_request
          machine_request:
            profile: /x/profile.yaml
            timeout: 30s
            request:
              body: {query: $.query}
            response:
              terminal_signals:
                Done: {status: 200}
`

// TestParseDefinitionAcceptsValidMachineRequest is the round-trip baseline: a
// machine_request using only implemented fields parses and validates (GH-486).
func TestParseDefinitionAcceptsValidMachineRequest(t *testing.T) {
	t.Parallel()
	_, err := ParseDefinition([]byte(validMachineRequestDoc))
	require.NoError(t, err)
}

// TestParseDefinitionRejectsUnknownMachineRequestField proves a documented-but-
// unimplemented field (here error_responses) is now rejected at load rather than
// silently ignored (GH-486).
func TestParseDefinitionRejectsUnknownMachineRequestField(t *testing.T) {
	t.Parallel()
	doc := `
rest:
  version: v1
  servers:
    srv:
      address: 127.0.0.1:0
      endpoints:
        e:
          method: POST
          path: /doc
          binding: machine_request
          machine_request:
            profile: /x/profile.yaml
            timeout: 30s
            error_responses:
              request_invalid: {status: 400}
            request:
              body: {query: $.query}
            response:
              terminal_signals:
                Done: {status: 200}
`
	_, err := ParseDefinition([]byte(doc))
	require.Error(t, err)
	require.Contains(t, err.Error(), "error_responses")
}

// TestParseDefinitionRejectsUnknownRequestMappingField proves the removed
// request-mapping metadata field is also rejected rather than parsed-and-ignored.
func TestParseDefinitionRejectsUnknownRequestMappingField(t *testing.T) {
	t.Parallel()
	doc := `
rest:
  version: v1
  servers:
    srv:
      address: 127.0.0.1:0
      endpoints:
        e:
          method: POST
          path: /doc
          binding: machine_request
          machine_request:
            profile: /x/profile.yaml
            timeout: 30s
            request:
              body: {query: $.query}
              metadata: [request_id]
            response:
              terminal_signals:
                Done: {status: 200}
`
	_, err := ParseDefinition([]byte(doc))
	require.Error(t, err)
	require.Contains(t, err.Error(), "metadata")
}
