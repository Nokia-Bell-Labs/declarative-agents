// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/catalog"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/spec"
)

func TestMain(m *testing.M) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("caller")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	spec.SetAgentCoreInstallRoot(root)
	os.Exit(m.Run())
}

func TestRESTOpenAPI_ImportAllowlist(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "payments.yaml"), paymentsOpenAPI(false))
	writeFile(t, filepath.Join(dir, "rest.yaml"), restOpenAPIConfig("payments.yaml"))

	def, err := LoadDefinition(filepath.Join(dir, "rest.yaml"))
	require.NoError(t, err)
	operations := def.Clients["payments"].Operations
	require.Contains(t, operations, "getPayment")
	require.Contains(t, operations, "createPayment")
	require.NotContains(t, operations, "cancelPayment")
	require.Equal(t, "GET", operations["getPayment"].Method)
	require.Equal(t, "/payments/{id}", operations["getPayment"].Path)
}

func TestRESTOpenAPI_ServerBindMap(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "payments.yaml"), paymentsOpenAPI(false))
	writeFile(t, filepath.Join(dir, "rest.yaml"), restOpenAPIConfig("payments.yaml"))

	def, err := LoadDefinition(filepath.Join(dir, "rest.yaml"))
	require.NoError(t, err)
	endpoint := def.Servers["payments_webhooks"].Endpoints["payment_webhook"]
	require.Equal(t, "POST", endpoint.Method)
	require.Equal(t, "/webhooks/payment", endpoint.Path)
	require.Equal(t, "emit_signal", endpoint.Binding)
	require.Contains(t, endpoint.Request.BodySchema, "properties")
}

func TestRESTOpenAPI_ServerBindPreservesEndpointConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "payments.yaml"), paymentsOpenAPI(false))
	writeFile(t, filepath.Join(dir, "rest.yaml"), preservingEndpointConfig("payments.yaml"))

	def, err := LoadDefinition(filepath.Join(dir, "rest.yaml"))
	require.NoError(t, err)
	endpoint := def.Servers["payments_webhooks"].Endpoints["payment_webhook"]
	require.Equal(t, "POST", endpoint.Method)
	require.Equal(t, "/webhooks/payment", endpoint.Path)
	require.Equal(t, "read_state", endpoint.Binding)
	require.Equal(t, "PaymentWebhookReceived", endpoint.Signal)
	require.ElementsMatch(t, []string{"PaymentWebhookReceived"}, endpoint.AllowedSignals)
	require.Equal(t, "machine_spec", endpoint.MonitorView)
	require.Equal(t, "payments", endpoint.Queue.Name)
	require.Equal(t, map[string]string{"accepted": "true"}, endpoint.Response.Output)
	require.Equal(t, []string{"body.secret"}, endpoint.Response.Redact)
}

func TestRESTOpenAPI_LoadsTrustedURLImport(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(paymentsOpenAPI(false)))
	}))
	t.Cleanup(server.Close)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "rest.yaml"), restOpenAPIConfig(server.URL))

	def, err := LoadDefinition(filepath.Join(dir, "rest.yaml"))
	require.NoError(t, err)
	require.Equal(t, "GET", def.Clients["payments"].Operations["getPayment"].Method)
}

func TestRESTOpenAPI_InvalidOperationIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		openapi string
		config  string
		wantErr string
	}{
		{name: "missing operation id", openapi: paymentsOpenAPI(false), config: missingOperationConfig(), wantErr: "missingPayment"},
		{name: "duplicate operation id", openapi: paymentsOpenAPI(true), config: restOpenAPIConfig("payments.yaml"), wantErr: "same operation id"},
		{name: "incompatible server binding", openapi: paymentsOpenAPI(false), config: incompatibleBindConfig(), wantErr: "incompatible"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, "payments.yaml"), tc.openapi)
			writeFile(t, filepath.Join(dir, "rest.yaml"), tc.config)

			_, err := LoadDefinition(filepath.Join(dir, "rest.yaml"))
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestRESTOpenAPI_LoadsOllamaProfileConfig(t *testing.T) {
	t.Parallel()

	root := profileRoot(t)
	def, err := LoadDefinition(filepath.Join(root, "rest/ollama-rest.yaml"))
	require.NoError(t, err)
	operation := def.Clients["ollama"].Operations["listOllamaModels"]
	require.Equal(t, "http://127.0.0.1:11434", def.Clients["ollama"].BaseURL)
	require.Equal(t, "GET", operation.Method)
	require.Equal(t, "/api/tags", operation.Path)
	require.NotContains(t, def.Clients["ollama"].Operations, "generate")

	profile, err := catalog.LoadProfile(filepath.Join(root, "rest/ollama-profile.yaml"))
	require.NoError(t, err)
	selection, err := catalog.LoadToolSelections(profile.Tools)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{
		"invoke_llm",
		"parse_response",
		"report_parse_error",
		"done",
		"ollama_list_models",
	}, selection)

	declarations, err := catalog.LoadToolDeclarations(profile.ToolDeclarations)
	require.NoError(t, err)
	selected, err := catalog.SelectTools(declarations, selection)
	require.NoError(t, err)
	restTool := requireSelectedTool(t, selected, "ollama_list_models")
	require.Equal(t, InitClientInvoke, restTool.Init)
	require.Equal(t, core.External, restTool.ToToolSpec().Visibility)
	requireNoAuthorityParameters(t, restTool.Parameters)
	requireConfigUsesNamedRefs(t, restTool)
	require.Equal(t, []string{"ollama_list_models"}, externalToolNames(selected))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func requireSelectedTool(t *testing.T, defs []catalog.ToolDef, name string) catalog.ToolDef {
	t.Helper()
	for _, def := range defs {
		if def.Name == name {
			return def
		}
	}
	t.Fatalf("selected tool %q not found", name)
	return catalog.ToolDef{}
}

func externalToolNames(defs []catalog.ToolDef) []string {
	names := []string{}
	for _, def := range defs {
		if def.ToToolSpec().Visibility == core.External {
			names = append(names, def.Name)
		}
	}
	return names
}

func repoRoot(t *testing.T) string {
	t.Helper()
	return repoRootFromRuntime()
}

func repoRootFromRuntime() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("resolve test file")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

func profileRoot(t *testing.T) string {
	t.Helper()
	for _, candidate := range profileRootCandidates(repoRoot(t)) {
		if hasProfile(candidate, "rest/ollama-profile.yaml") {
			return candidate
		}
		nested := filepath.Join(candidate, "agents")
		if hasProfile(nested, "rest/ollama-profile.yaml") {
			return nested
		}
	}
	t.Fatalf("profile root not found; place agent-profiles next to agent-core or under ./agent-profiles")
	return ""
}

func profileRootCandidates(root string) []string {
	return []string{
		filepath.Join(filepath.Dir(root), "agent-profiles"),
		filepath.Join(root, "agent-profiles"),
	}
}

func hasProfile(root, rel string) bool {
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil && !info.IsDir()
}

func paymentsOpenAPI(duplicate bool) string {
	extra := ""
	if duplicate {
		extra = `
  /duplicates/{id}:
    get:
      operationId: getPayment
      parameters:
        - name: id
          in: path
          required: true
          schema: {type: string}
      responses:
        "200":
          description: duplicate
`
	}
	return `openapi: 3.0.3
info: {title: Payments, version: v1}
paths:
  /payments/{id}:
    get:
      operationId: getPayment
      parameters:
        - name: id
          in: path
          required: true
          schema: {type: string}
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                type: object
                properties:
                  id: {type: string}
    post:
      operationId: createPayment
      parameters:
        - name: id
          in: path
          required: true
          schema: {type: string}
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                amount: {type: number}
      responses:
        "202":
          description: accepted
  /payments/{id}/cancel:
    post:
      operationId: cancelPayment
      parameters:
        - name: id
          in: path
          required: true
          schema: {type: string}
      responses:
        "202":
          description: accepted
  /webhooks/payment:
    post:
      operationId: receivePaymentWebhook
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                id: {type: string}
      responses:
        "202":
          description: accepted
` + extra
}

func restOpenAPIConfig(path string) string {
	return `rest:
  version: v1
  openapi:
    payments:
      path: ` + path + `
      base_url: https://payments.internal
      expose: [getPayment, createPayment]
      bind:
        receivePaymentWebhook: payment_webhook
      side_effects:
        createPayment:
          - kind: external_api
            target: payments.payment
            state: payment_created
      reversibility:
        createPayment:
          classification: compensatable
          undo: cancelPayment
  servers:
    payments_webhooks:
      address: 127.0.0.1:0
      endpoints:
        payment_webhook:
          binding: emit_signal
          signal: PaymentWebhookReceived
`
}

func preservingEndpointConfig(path string) string {
	return `rest:
  version: v1
  openapi:
    payments:
      path: ` + path + `
      bind:
        receivePaymentWebhook: payment_webhook
  servers:
    payments_webhooks:
      address: 127.0.0.1:0
      endpoints:
        payment_webhook:
          binding: read_state
          signal: PaymentWebhookReceived
          allowed_signals: [PaymentWebhookReceived]
          monitor_view: machine_spec
          queue:
            name: payments
          response:
            output:
              accepted: "true"
            redact: [body.secret]
`
}

func missingOperationConfig() string {
	return `rest:
  version: v1
  openapi:
    payments:
      path: payments.yaml
      expose: [missingPayment]
`
}

func incompatibleBindConfig() string {
	return `rest:
  version: v1
  openapi:
    payments:
      path: payments.yaml
      bind:
        receivePaymentWebhook: payment_webhook
  servers:
    payments_webhooks:
      address: 127.0.0.1:0
      endpoints:
        payment_webhook:
          openapi_operation_id: getPayment
          binding: emit_signal
          signal: PaymentWebhookReceived
`
}
