// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateDefinitionRejectsConfigFormatRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(Definition) Definition
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
		mutate  func(Definition) Definition
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

func clearVersion(def Definition) Definition {
	def.Version = ""
	return def
}

func undeclaredBodyParam(def Definition) Definition {
	op := def.Clients["github"].Resources["issue"].Operations["set"]
	op.Body["title"] = "{{ params.missing }}"
	def.Clients["github"].Resources["issue"].Operations["set"] = op
	return def
}

func unsupportedResourceVerb(def Definition) Definition {
	resource := def.Clients["github"].Resources["issue"]
	resource.Operations["approve"] = validWriteOperation()
	def.Clients["github"].Resources["issue"] = resource
	return def
}

func missingSideEffects(def Definition) Definition {
	op := validWriteOperation()
	op.SideEffects = nil
	def.Clients["github"].Operations["mutate"] = op
	return def
}

func missingReversibility(def Definition) Definition {
	op := validWriteOperation()
	op.Reversibility = Reversibility{}
	def.Clients["github"].Operations["mutate"] = op
	return def
}

func asyncMissingRequestID(def Definition) Definition {
	op := validWriteOperation()
	op.Async = &AsyncClientConfig{Timeout: "10s"}
	def.Clients["github"].Operations["async_mutate"] = op
	return def
}

func asyncMissingTimeout(def Definition) Definition {
	op := validWriteOperation()
	op.Async = &AsyncClientConfig{RequestID: "$.id"}
	def.Clients["github"].Operations["async_mutate"] = op
	return def
}

func dynamicSignalNoAllowlist(def Definition) Definition {
	endpoint := def.Servers["control"].Endpoints["approve"]
	endpoint.Binding = bindingDynamicSignal
	endpoint.AllowedSignals = nil
	def.Servers["control"].Endpoints["approve"] = endpoint
	return def
}

func publicListener(def Definition) Definition {
	server := def.Servers["control"]
	server.Address = "0.0.0.0:8080"
	def.Servers["control"] = server
	return def
}

func unsupportedAuth(def Definition) Definition {
	def.Auth["github_app"] = AuthProfile{Type: "magic_signature"}
	return def
}

func unsupportedRedirect(def Definition) Definition {
	limit := def.Limits["public_api"]
	limit.Redirect.Mode = "anywhere"
	def.Limits["public_api"] = limit
	return def
}

func invalidRedaction(def Definition) Definition {
	op := def.Clients["github"].Operations["search_issues"]
	op.Response.Redact = []string{"secret"}
	def.Clients["github"].Operations["search_issues"] = op
	return def
}

func duplicateImportedOperation(def Definition) Definition {
	def.OpenAPI = map[string]OpenAPIImport{"github": {Expose: []string{"search_issues"}}}
	return def
}

func duplicateImportedEndpoint(def Definition) Definition {
	def.OpenAPI = map[string]OpenAPIImport{"control": {Bind: map[string]string{"approveOp": "approve"}}}
	return def
}

func baseDefinition() Definition {
	return Definition{
		Version: "v1",
		Auth: map[string]AuthProfile{
			"github_app": {Type: authBearer, TokenRef: "github_token"},
		},
		Limits: map[string]LimitProfile{
			"public_api": {Redirect: RedirectPolicy{Mode: redirectSameHost}},
		},
		Clients: map[string]Client{"github": baseClient()},
		Servers: map[string]Server{"control": {
			Address: "127.0.0.1:0",
			Endpoints: map[string]Endpoint{
				"approve": validEndpoint(),
			},
		}},
	}
}

func baseClient() Client {
	return Client{
		BaseURL:   "https://api.github.com",
		AuthRef:   "github_app",
		LimitsRef: "public_api",
		Resources: map[string]Resource{"issue": {
			Path: "/repos/{owner}/{repo}/issues/{number}",
			Operations: map[string]Operation{
				"get": validReadOperation(),
				"set": validWriteOperation(),
			},
		}},
		Operations: map[string]Operation{"search_issues": validReadOperation()},
	}
}

func validReadOperation() Operation {
	return Operation{
		Method: "GET",
		Path:   "/search/issues",
		Params: pathBinding(),
		Success: StatusMapping{
			Status: []int{200},
			Signal: "RESTResourceRead",
		},
		Response: ResponseMapping{Redact: []string{"headers.authorization"}},
	}
}

func validWriteOperation() Operation {
	op := validReadOperation()
	op.Method = "PATCH"
	op.Body = map[string]interface{}{"title": "{{ params.title }}"}
	op.Params.BodySchema = bodySchema("title")
	op.SideEffects = []SideEffect{{Kind: "external_api", Target: "github.issue"}}
	op.Reversibility = Reversibility{Classification: "compensatable", Undo: "restore"}
	return op
}

func validEndpoint() Endpoint {
	return Endpoint{
		Method:  "POST",
		Path:    "/approve/{id}",
		Binding: "emit_signal",
		Signal:  "Approved",
		Request: RequestBinding{Path: map[string]interface{}{
			"id": map[string]interface{}{"type": "string"},
		}},
		Response: ResponseMapping{Redact: []string{"body.secret"}},
	}
}

func pathBinding() RequestBinding {
	return RequestBinding{Path: map[string]interface{}{
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
