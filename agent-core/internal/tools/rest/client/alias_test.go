// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package client_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	toolrest "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
	restclient "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/client"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/credentials"
	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/redact"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/resttest"
)

type (
	Definition                = restdef.Definition
	Client                    = restdef.Client
	Resource                  = restdef.Resource
	Operation                 = restdef.Operation
	StatusMapping             = restdef.StatusMapping
	AuthProfile               = restdef.AuthProfile
	LimitProfile              = restdef.LimitProfile
	RedirectPolicy            = restdef.RedirectPolicy
	NetworkPolicy             = restdef.NetworkPolicy
	RetryPolicy               = restdef.RetryPolicy
	RequestBinding            = restdef.RequestBinding
	ResponseMapping           = restdef.ResponseMapping
	AsyncClientConfig         = restdef.AsyncClientConfig
	SideEffect                = restdef.SideEffect
	Reversibility             = restdef.Reversibility
	ClientOperationDefinition = toolrest.ClientOperationDefinition
	ClientToolConfig          = toolrest.ClientToolConfig
	ClientBuilder             = toolrest.ClientBuilder
	CompensationExecutor      = toolrest.CompensationExecutor
	AsyncState                = toolrest.AsyncState
	AsyncRequest              = toolrest.AsyncRequest
	CredentialResolver        = credentials.Resolver
	StaticCredentials         = credentials.Static
	Collection                = toolrest.Collection
)

const (
	InitClientGet    = toolrest.InitClientGet
	InitClientSet    = toolrest.InitClientSet
	InitClientCreate = toolrest.InitClientCreate
	InitClientDelete = toolrest.InitClientDelete
	InitClientInvoke = toolrest.InitClientInvoke
	InitClientSend   = toolrest.InitClientSend
	InitClientAwait  = toolrest.InitClientAwait

	authNone        = "none"
	authBasic       = "basic"
	authBearer      = "bearer"
	authHeaderToken = "header_token"
	authQueryToken  = "query_token"

	bodySourceNone           = "none"
	bodySourceParams         = "params"
	bodySourcePreviousResult = "previous_result"
	bodySourceCommandState   = "command_state"

	redirectNone      = "none"
	redirectSameHost  = "same_host"
	redirectAllowlist = "allowlist"

	redactedValue         = redact.Value
	asyncRetentionConsume = "consume"
)

func clientCommand(t *testing.T, def Definition, init, operation string, input map[string]interface{}) core.Command {
	t.Helper()
	return clientCommandWithCredentials(t, def, init, operation, input, nil)
}

func clientCommandWithCredentials(
	t *testing.T,
	def Definition,
	init string,
	operation string,
	input map[string]interface{},
	credentials CredentialResolver,
) core.Command {
	t.Helper()
	return clientCommandWithMetricsAndCredentials(t, def, init, operation, input, restMetrics(), credentials)
}

func clientCommandWithMetrics(
	t *testing.T,
	def Definition,
	init string,
	operation string,
	input map[string]interface{},
	metrics core.MetricConfig,
) core.Command {
	t.Helper()
	return clientCommandWithMetricsAndCredentials(t, def, init, operation, input, metrics, nil)
}

func clientCommandWithMetricsAndCredentials(
	t *testing.T,
	def Definition,
	init string,
	operation string,
	input map[string]interface{},
	metrics core.MetricConfig,
	credentials CredentialResolver,
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
		ToolName: init, Init: init, Operation: resolved, Credentials: credentials, Metrics: metrics,
	}.Build(core.Result{Output: string(params)})
}

func restMetrics() core.MetricConfig {
	return core.MetricConfig{
		Instruments: []core.MetricInstrument{
			{Name: "rest.http_status_code", Kind: "gauge", Unit: "1", Description: "HTTP status.", ValueSource: "http_status_code", Attributes: []string{"operation"}},
			{Name: "rest.retry_count", Kind: "counter", Unit: "{retry}", Description: "Retry count.", ValueSource: "retry_count", Attributes: []string{"operation"}},
			{Name: "rest.request_bytes", Kind: "histogram", Unit: "By", Description: "Request bytes.", ValueSource: "request_bytes", Attributes: []string{"operation"}},
			{Name: "rest.response_bytes", Kind: "histogram", Unit: "By", Description: "Response bytes.", ValueSource: "response_bytes", Attributes: []string{"operation"}},
		},
		Attributes: []core.MetricAttribute{{Name: "operation", Source: "configured_operation", Cardinality: "bounded", AllowedValues: []string{"get"}, Redaction: "none"}},
	}
}

func clientDefinition(t *testing.T, baseURL string, client Client) Definition {
	t.Helper()
	return resttest.ClientDefinition(t, baseURL, client)
}

func requireClientSignal(t *testing.T, def Definition, init, operation string, input map[string]interface{}, signal string) {
	t.Helper()
	result := clientCommand(t, def, init, operation, input).Execute()
	require.Equal(t, core.Signal(signal), result.Signal, result.Output)
	require.Contains(t, result.Output, `"operation":"`+operation+`"`)
}

func ValidateDefinition(def Definition) error {
	return toolrest.ValidateDefinition(def)
}

func NewCollection() Collection {
	return toolrest.NewCollection()
}

func NewAsyncState() *AsyncState {
	return toolrest.NewAsyncState()
}

func writeJSON(w http.ResponseWriter, status int, payload map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func jsonOutput(value map[string]interface{}) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(data)
}

func commandStateDigest(output string) core.ResultDigest {
	return core.ResultDigest{
		Output:           output,
		RedactionVersion: core.OutputRedactionVersion1,
		RedactionStatus:  core.OutputRedactionApplied,
	}
}

func retryDelay(retry RetryPolicy, attempt int) time.Duration {
	return restclient.RetryDelay(retry, attempt)
}

func safeLabels(labels map[string]string) map[string]string {
	return redact.SafeLabels(labels)
}

func pathSegments(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func bodySchema(field string) map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			field: map[string]interface{}{"type": "string"},
		},
	}
}

func bodySchemaWithRequired(field string) map[string]interface{} {
	return map[string]interface{}{
		"type": "object", "required": []interface{}{field},
		"properties": map[string]interface{}{field: map[string]interface{}{"type": "string"}},
	}
}
