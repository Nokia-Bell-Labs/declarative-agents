// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"testing"

	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
	"github.com/stretchr/testify/require"
)

func TestValidateDefinitionRejectsConfigFormatRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(restdef.Definition) restdef.Definition
		wantErr string
	}{
		{name: "missing version", mutate: clearVersion, wantErr: "rest.version"},
		{name: "undeclared body param", mutate: undeclaredBodyParam, wantErr: "undeclared param"},
		{name: "unsupported resource verb", mutate: unsupportedResourceVerb, wantErr: "unsupported operation"},
		{name: "mutating operation missing side effects", mutate: missingSideEffects, wantErr: "side_effects"},
		{name: "mutating operation missing reversibility", mutate: missingReversibility, wantErr: "reversibility"},
		{name: "async missing request id", mutate: asyncMissingRequestID, wantErr: "request_id"},
		{name: "async missing timeout", mutate: asyncMissingTimeout, wantErr: "timeout"},
		{name: "dynamic signal without allowlist", mutate: dynamicSignalNoAllowlist, wantErr: "allowed_signals"},
		{name: "public listener rejected", mutate: publicListener, wantErr: "allow_public_listener"},
		{name: "unsupported auth type", mutate: unsupportedAuth, wantErr: "unsupported type"},
		{name: "unsupported redirect mode", mutate: unsupportedRedirect, wantErr: "redirect mode"},
		{name: "invalid redaction selector", mutate: invalidRedaction, wantErr: "redaction selector"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.ErrorContains(t, ValidateDefinition(tc.mutate(baseDefinition())), tc.wantErr)
		})
	}
}

func TestValidateDefinitionRejectsMergedNameCollisions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(restdef.Definition) restdef.Definition
		wantErr string
	}{
		{name: "operation import collision", mutate: duplicateImportedOperation, wantErr: "search_issues"},
		{name: "endpoint bind collision", mutate: duplicateImportedEndpoint, wantErr: "approve"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.ErrorContains(t, ValidateDefinition(tc.mutate(baseDefinition())), tc.wantErr)
		})
	}
}

func TestValidateDefinitionRejectsAmbiguousStatusMappings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		success  []int
		failures [][]int
		valid    bool
	}{
		{name: "disjoint exact statuses", success: []int{200, 201}, failures: [][]int{{404}, {422, 500}}, valid: true},
		{name: "success overlaps failure", success: []int{200}, failures: [][]int{{200}}},
		{name: "failure mappings overlap", success: []int{200}, failures: [][]int{{404, 422}, {422, 500}}},
		{name: "reversed failures still overlap", success: []int{200}, failures: [][]int{{422, 500}, {404, 422}}},
		{name: "duplicate success status", success: []int{200, 200}},
		{name: "duplicate within failure", success: []int{200}, failures: [][]int{{404, 404}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			def := baseDefinition()
			def.Clients["github"].Operations["search_issues"] = operationWithStatusMappings(tt.success, tt.failures...)
			err := ValidateDefinition(def)
			if tt.valid {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, "maps HTTP status")
			require.ErrorContains(t, err, "more than once")
		})
	}
}

func TestValidateStatusMappingsEveryHTTPCodeHasOneOwner(t *testing.T) {
	t.Parallel()
	for status := 100; status <= 599; status++ {
		other := status + 1
		if other > 599 {
			other = 100
		}
		require.NoError(t, validateStatusMappings("probe", operationWithStatusMappings([]int{status}, []int{other})))
		require.Error(t, validateStatusMappings("probe", operationWithStatusMappings([]int{status}, []int{status})))
	}
}

func operationWithStatusMappings(success []int, failures ...[]int) restdef.Operation {
	op := validReadOperation()
	op.Success = restdef.StatusMapping{Status: success, Signal: "RESTDone"}
	for index, statuses := range failures {
		signal := "RESTFailed"
		if index > 0 {
			signal = "RESTDomainFailed"
		}
		op.Failures = append(op.Failures, restdef.StatusMapping{Status: statuses, Signal: signal})
	}
	return op
}

func clearVersion(def restdef.Definition) restdef.Definition {
	def.Version = ""
	return def
}

func undeclaredBodyParam(def restdef.Definition) restdef.Definition {
	op := def.Clients["github"].Resources["issue"].Operations["set"]
	op.Body["title"] = "{{ params.missing }}"
	def.Clients["github"].Resources["issue"].Operations["set"] = op
	return def
}

func unsupportedResourceVerb(def restdef.Definition) restdef.Definition {
	resource := def.Clients["github"].Resources["issue"]
	resource.Operations["approve"] = validWriteOperation()
	def.Clients["github"].Resources["issue"] = resource
	return def
}

func missingSideEffects(def restdef.Definition) restdef.Definition {
	op := validWriteOperation()
	op.SideEffects = nil
	def.Clients["github"].Operations["mutate"] = op
	return def
}

func missingReversibility(def restdef.Definition) restdef.Definition {
	op := validWriteOperation()
	op.Reversibility = restdef.Reversibility{}
	def.Clients["github"].Operations["mutate"] = op
	return def
}

func asyncMissingRequestID(def restdef.Definition) restdef.Definition {
	op := validWriteOperation()
	op.Async = &restdef.AsyncClientConfig{Timeout: "10s"}
	def.Clients["github"].Operations["async_mutate"] = op
	def.Clients["github"].Operations["set"] = validWriteOperation()
	return def
}

func asyncMissingTimeout(def restdef.Definition) restdef.Definition {
	op := validWriteOperation()
	op.Async = &restdef.AsyncClientConfig{RequestID: "$.id"}
	def.Clients["github"].Operations["async_mutate"] = op
	def.Clients["github"].Operations["set"] = validWriteOperation()
	return def
}

func dynamicSignalNoAllowlist(def restdef.Definition) restdef.Definition {
	endpoint := def.Servers["control"].Endpoints["approve"]
	endpoint.Binding = bindingDynamicSignal
	endpoint.AllowedSignals = nil
	def.Servers["control"].Endpoints["approve"] = endpoint
	return def
}

func publicListener(def restdef.Definition) restdef.Definition {
	server := def.Servers["control"]
	server.Address = "0.0.0.0:8080"
	def.Servers["control"] = server
	return def
}

func unsupportedAuth(def restdef.Definition) restdef.Definition {
	def.Auth["github_app"] = restdef.AuthProfile{Type: "magic_signature"}
	return def
}

func TestValidateDefinitionRejectsReservedResourceMetadataFields(t *testing.T) {
	t.Parallel()
	def := baseDefinition()
	resource := def.Clients["github"].Resources["issue"]
	resource.IDField = "id"
	client := def.Clients["github"]
	client.Resources["issue"] = resource
	def.Clients["github"] = client
	require.ErrorContains(t, ValidateDefinition(def), "id_field and version_field are reserved")
}

func unsupportedRedirect(def restdef.Definition) restdef.Definition {
	limit := def.Limits["public_api"]
	limit.Redirect.Mode = "anywhere"
	def.Limits["public_api"] = limit
	return def
}

func invalidRedaction(def restdef.Definition) restdef.Definition {
	op := def.Clients["github"].Operations["search_issues"]
	op.Response.Redact = []string{"secret"}
	def.Clients["github"].Operations["search_issues"] = op
	return def
}

func duplicateImportedOperation(def restdef.Definition) restdef.Definition {
	def.OpenAPI = map[string]restdef.OpenAPIImport{"github": {Expose: []string{"search_issues"}}}
	return def
}

func duplicateImportedEndpoint(def restdef.Definition) restdef.Definition {
	def.OpenAPI = map[string]restdef.OpenAPIImport{"control": {Bind: map[string]string{"approveOp": "approve"}}}
	return def
}

func baseDefinition() restdef.Definition {
	return restdef.Definition{
		Version: "v1",
		Auth: map[string]restdef.AuthProfile{
			"github_app": {Type: authBearer, TokenRef: "github_token"},
		},
		Limits: map[string]restdef.LimitProfile{
			"public_api": {Redirect: restdef.RedirectPolicy{Mode: redirectSameHost}},
		},
		Clients: map[string]restdef.Client{"github": baseClient()},
		Servers: map[string]restdef.Server{"control": {
			Address: "127.0.0.1:0",
			Endpoints: map[string]restdef.Endpoint{
				"approve": validEndpoint(),
			},
		}},
	}
}

func baseClient() restdef.Client {
	return restdef.Client{
		BaseURL:   "https://api.github.com",
		AuthRef:   "github_app",
		LimitsRef: "public_api",
		Resources: map[string]restdef.Resource{"issue": {
			Path: "/repos/{owner}/{repo}/issues/{number}",
			Operations: map[string]restdef.Operation{
				"get": validReadOperation(),
				"set": validWriteOperation(),
			},
		}},
		Operations: map[string]restdef.Operation{"search_issues": validReadOperation()},
	}
}

func validReadOperation() restdef.Operation {
	return restdef.Operation{
		Method: "GET",
		Path:   "/search/issues",
		Params: pathBinding(),
		Success: restdef.StatusMapping{
			Status: []int{200},
			Signal: "RESTResourceRead",
		},
		Response: restdef.ResponseMapping{Redact: []string{"headers.authorization"}},
	}
}

func validWriteOperation() restdef.Operation {
	op := validReadOperation()
	op.Method = "PATCH"
	op.Body = map[string]interface{}{"title": "{{ params.title }}"}
	op.Params.BodySchema = bodySchema("title")
	op.SideEffects = []restdef.SideEffect{{Kind: "external_api", Target: "github.issue"}}
	op.Reversibility = restdef.Reversibility{Classification: "compensatable", Undo: "restore"}
	op.Compensation = map[string]interface{}{
		"operation": "set",
		"parameters": map[string]interface{}{
			"title": "restored",
		},
	}
	return op
}

func validEndpoint() restdef.Endpoint {
	return restdef.Endpoint{
		Method:  "POST",
		Path:    "/approve/{id}",
		Binding: "emit_signal",
		Signal:  "Approved",
		Request: restdef.RequestBinding{Path: map[string]interface{}{
			"id": map[string]interface{}{"type": "string"},
		}},
		Response: restdef.ResponseMapping{Redact: []string{"body.secret"}},
	}
}

func pathBinding() restdef.RequestBinding {
	return restdef.RequestBinding{Path: map[string]interface{}{
		"owner":  map[string]interface{}{"type": "string"},
		"repo":   map[string]interface{}{"type": "string"},
		"number": map[string]interface{}{"type": "integer"},
	}}
}

func bodySchema(field string) map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			field: map[string]interface{}{"type": "string"},
		},
	}
}

func validStaticAssetsEndpoint() restdef.Endpoint {
	return restdef.Endpoint{
		Method:  "GET",
		Path:    "/ui/{path...}",
		Binding: bindingStaticAssets,
		StaticAssets: &restdef.StaticAssetsConfig{
			Root: "/tmp/static-root",
		},
		Request: restdef.RequestBinding{Path: map[string]interface{}{
			"path": map[string]interface{}{"type": "string"},
		}},
	}
}

func singleServerDefinition(ep restdef.Endpoint) restdef.Definition {
	return restdef.Definition{
		Version: "v1",
		Servers: map[string]restdef.Server{
			"srv": {
				Address: "127.0.0.1:0",
				Endpoints: map[string]restdef.Endpoint{
					"e": ep,
				},
			},
		},
	}
}

func TestValidateDefinition_staticAssetsRejectsInvalidConfigs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ep      restdef.Endpoint
		wantErr string
	}{
		{
			name: "wrong http method",
			ep: func() restdef.Endpoint {
				e := validStaticAssetsEndpoint()
				e.Method = "POST"
				return e
			}(),
			wantErr: "requires GET method",
		},
		{
			name: "blank assets root",
			ep: func() restdef.Endpoint {
				e := validStaticAssetsEndpoint()
				e.StaticAssets = &restdef.StaticAssetsConfig{Root: "  "}
				return e
			}(),
			wantErr: "non-empty root",
		},
		{
			name: "missing static_assets block",
			ep: func() restdef.Endpoint {
				e := validStaticAssetsEndpoint()
				e.StaticAssets = nil
				return e
			}(),
			wantErr: "requires static_assets config",
		},
		{
			name: "signal conflicts with static binding",
			ep: func() restdef.Endpoint {
				e := validStaticAssetsEndpoint()
				e.Signal = "Noise"
				return e
			}(),
			wantErr: "must not set signal",
		},
		{
			name: "static_assets config with wrong binding",
			ep: func() restdef.Endpoint {
				e := validStaticAssetsEndpoint()
				e.Binding = bindingEmitSignal
				e.Signal = "Y"
				return e
			}(),
			wantErr: "static_assets config but binding",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.ErrorContains(t, ValidateDefinition(singleServerDefinition(tc.ep)), tc.wantErr)
		})
	}
}
