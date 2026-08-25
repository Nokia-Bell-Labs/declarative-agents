// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package resttest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	toolrest "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
)

// IssueHandler serves the canned GitHub-issue fixture used by client tests.
func IssueHandler(w http.ResponseWriter, req *http.Request) {
	switch req.URL.Path {
	case "/repos/acme/agent-core/issues/boom":
		http.Error(w, "boom", http.StatusInternalServerError)
		return
	case "/repos/acme/agent-core/issues/missing":
		http.NotFound(w, req)
		return
	case "/repos/acme/agent-core/issues/domain":
		http.Error(w, `{"error":"domain"}`, http.StatusUnprocessableEntity)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"title": "ok", "id": "ISS-1", "request_id": "REQ-1"})
}

// IssueClient is the shared GitHub-issue REST client fixture.
func IssueClient() toolrest.Client {
	return toolrest.Client{Resources: map[string]toolrest.Resource{"issue": {
		Path: "/repos/{owner}/{repo}/issues/{number}",
		Operations: map[string]toolrest.Operation{
			"get": IssueOperation(http.MethodGet, "RESTResourceRead"),
			"set": issueSetOperation(),
		},
	}}}
}

// ClientDefinition builds a validated client definition against baseURL.
func ClientDefinition(t *testing.T, baseURL string, client toolrest.Client) toolrest.Definition {
	t.Helper()
	client.BaseURL = baseURL
	client.AuthRef = "none"
	def := toolrest.Definition{
		Version: "v1",
		Auth:    map[string]toolrest.AuthProfile{"none": {Type: "none"}},
		Limits:  map[string]toolrest.LimitProfile{"test": {}},
		Clients: map[string]toolrest.Client{"github": client},
	}
	require.NoError(t, toolrest.ValidateDefinition(def))
	return def
}

// ClientCommand builds a REST client command against the github/issue fixture.
func ClientCommand(t *testing.T, def toolrest.Definition, init, operation string, input map[string]interface{}) core.Command {
	t.Helper()
	return ClientCommandWithCredentials(t, def, init, operation, input, nil)
}

// ClientCommandWithCredentials builds a REST client command with a credential resolver.
func ClientCommandWithCredentials(
	t *testing.T,
	def toolrest.Definition,
	init string,
	operation string,
	input map[string]interface{},
	credentials toolrest.CredentialResolver,
) core.Command {
	t.Helper()
	collection := toolrest.NewCollection()
	require.NoError(t, collection.Add(def))
	resolved, err := collection.ResolveClientOperation(toolrest.ClientToolConfig{
		RestRef: "github", Resource: "issue", Operation: operation,
	})
	require.NoError(t, err)
	params, err := json.Marshal(map[string]interface{}{"tool": init, "parameters": input})
	require.NoError(t, err)
	return toolrest.ClientBuilder{
		ToolName: init, Init: init, Operation: resolved, Credentials: credentials,
	}.Build(core.Result{Output: string(params)})
}

// IssueOperation is one GitHub-issue REST operation fixture.
func IssueOperation(method, signal string) toolrest.Operation {
	return toolrest.Operation{
		Method: method,
		Params: toolrest.RequestBinding{Path: map[string]interface{}{
			"owner": map[string]interface{}{}, "repo": map[string]interface{}{}, "number": map[string]interface{}{},
		}},
		Success:  toolrest.StatusMapping{Status: []int{200}, Signal: signal},
		Failures: []toolrest.StatusMapping{{Status: []int{404}, Signal: "RESTMissing"}, {Status: []int{422}, Signal: "RESTDomainFailed"}},
		Response: toolrest.ResponseMapping{
			Output: map[string]string{"title": "$.title"}, Redact: []string{"body.secret"},
			ResourceID: "$.id", RequestID: "$.request_id",
		},
		SideEffects:   []toolrest.SideEffect{{Kind: "external_api", State: "read_only"}},
		Reversibility: toolrest.Reversibility{Classification: "reversible", Undo: "noop"},
	}
}

func issueSetOperation() toolrest.Operation {
	op := IssueOperation(http.MethodPatch, "RESTResourceWritten")
	op.Params.BodySchema = bodySchema("title")
	op.Body = map[string]interface{}{"title": "{{ params.title }}"}
	op.SideEffects = []toolrest.SideEffect{{Kind: "external_api", State: "issue_updated"}}
	op.Reversibility = toolrest.Reversibility{Classification: "compensatable", Undo: "restore"}
	op.Compensation = map[string]interface{}{
		"operation":  "set",
		"parameters": map[string]interface{}{"title": "restored"},
	}
	return op
}

func bodySchema(field string) map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			field: map[string]interface{}{"type": "string"},
		},
	}
}

// Params builds github/issue path parameters, with an optional title.
func Params(number string, title ...string) map[string]interface{} {
	values := map[string]interface{}{"owner": "acme", "repo": "agent-core", "number": number}
	if len(title) > 0 {
		values["title"] = title[0]
	}
	return values
}

// LoopbackCIDR returns the CIDR that covers a loopback httptest host.
func LoopbackCIDR(server *httptest.Server) string {
	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	if strings.Contains(req.URL.Hostname(), ":") {
		return "::1/128"
	}
	return "127.0.0.0/8"
}

// AuthCredentials is the in-memory credential map used by client tests.
func AuthCredentials() toolrest.StaticCredentials {
	return toolrest.StaticCredentials{
		"github_token": "synthetic-token",
		"user_ref":     "synthetic-user",
		"password_ref": "synthetic-password",
		"token_ref":    "synthetic-token",
	}
}
