// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseDefinitionLoadsValidHandAuthoredConfig(t *testing.T) {
	t.Parallel()

	def, err := ParseDefinition([]byte(validDefinitionYAML))
	require.NoError(t, err)
	require.Equal(t, "v1", def.Version)
	require.Equal(t, "https://api.github.com", def.Clients["github"].BaseURL)
	require.Contains(t, def.Clients["github"].Resources["issue"].Operations, "get")
	require.Equal(t, "127.0.0.1:0", def.Servers["control"].Address)
}

const validDefinitionYAML = `
rest:
  version: v1
  auth:
    github_app:
      type: bearer
      token_ref: github_token
  limits:
    public_api:
      timeout: 30s
      redirect:
        mode: same_host
  clients:
    github:
      base_url: https://api.github.com
      auth_ref: github_app
      limits_ref: public_api
      resources:
        issue:
          path: /repos/{owner}/{repo}/issues/{number}
          operations:
            get:
              method: GET
              params:
                path:
                  owner: {type: string}
                  repo: {type: string}
                  number: {type: integer}
              success: {status: [200], signal: RESTResourceRead}
              response:
                output:
                  title: $.title
              side_effects:
                - kind: external_api
                  target: github.issue
                  state: read_only
              reversibility:
                classification: reversible
                undo: noop
            set:
              method: PATCH
              params:
                path:
                  owner: {type: string}
                  repo: {type: string}
                  number: {type: integer}
                body_schema:
                  type: object
                  properties:
                    title: {type: string}
              body:
                title: "{{ params.title }}"
              success: {status: [200], signal: RESTResourceWritten}
              side_effects:
                - kind: external_api
                  target: github.issue
                  state: issue_updated
              reversibility:
                classification: compensatable
                undo: restore_previous_issue_fields
      operations:
        search_issues:
          method: GET
          path: /search/issues
          params:
            query:
              q: {type: string}
          success: {status: [200], signal: RESTResponded}
          side_effects:
            - kind: external_api
              target: github.search
              state: read_only
          reversibility:
            classification: reversible
            undo: noop
  servers:
    control:
      address: 127.0.0.1:0
      endpoints:
        approve:
          method: POST
          path: /approve/{id}
          binding: emit_signal
          signal: Approved
          request:
            path:
              id: {type: string}
          response:
            output:
              accepted: "true"
`
