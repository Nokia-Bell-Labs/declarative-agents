// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

func TestRESTClient_SendRecordsAsyncRequest(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(asyncPaymentHandler))
	defer upstream.Close()
	def := asyncDefinition(t, upstream.URL, asyncPaymentClient())
	state := NewAsyncState()

	start := time.Now()
	result := asyncCommand(def, state, InitClientSend, asyncParams("slow")).Execute()
	require.Equal(t, core.Signal("RESTAccepted"), result.Signal, result.Output)
	require.Less(t, time.Since(start), 50*time.Millisecond)
	require.Contains(t, result.Output, `"request_id":"slow"`)
	require.Contains(t, result.Output, `"idempotency_token":"slow"`)

	await := asyncCommand(def, state, InitClientAwait, map[string]interface{}{"request_id": "slow"}).Execute()
	require.Equal(t, core.Signal("RESTResponded"), await.Signal, await.Output)
}

func TestRESTClient_AwaitAsyncRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		id     string
		signal core.Signal
	}{
		{name: "responded", id: "ok", signal: core.Signal("RESTResponded")},
		{name: "domain failed", id: "domain", signal: core.Signal("RESTDomainFailed")},
		{name: "missing", id: "missing", signal: core.Signal("RESTMissing")},
		{name: "timed out", id: "timeout", signal: core.Signal("RESTAwaitTimedOut")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(asyncPaymentHandler))
			defer upstream.Close()
			def := asyncDefinition(t, upstream.URL, asyncPaymentClient())
			state := NewAsyncState()
			send := asyncCommand(def, state, InitClientSend, asyncParams(tc.id)).Execute()
			require.Equal(t, core.Signal("RESTAccepted"), send.Signal, send.Output)

			await := asyncCommand(def, state, InitClientAwait, map[string]interface{}{"request_id": tc.id}).Execute()
			require.Equal(t, tc.signal, await.Signal, await.Output)
		})
	}

	state := NewAsyncState()
	def := asyncDefinition(t, "http://127.0.0.1:1", asyncPaymentClient())
	result := asyncCommand(def, state, InitClientAwait, map[string]interface{}{"request_id": "missing"}).Execute()
	require.Equal(t, core.CommandError, result.Signal)
	require.Contains(t, result.Output, "async_state_missing")
}

func TestRESTClient_AsyncRetryPolicyValidation(t *testing.T) {
	t.Parallel()

	def := asyncDefinition(t, "https://api.example", asyncPaymentClient())
	def.RetryPolicies = map[string]RetryPolicy{"retry": {
		Attempts: 2, RetryStatus: []int{429}, RequireIdempotency: true,
	}}
	client := def.Clients["payments"]
	client.RetryRef = "retry"
	def.Clients["payments"] = client
	require.NoError(t, ValidateDefinition(def))

	op := def.Clients["payments"].Operations["create_payment"]
	op.Async.IdempotencyToken = ""
	def.Clients["payments"].Operations["create_payment"] = op
	require.ErrorContains(t, ValidateDefinition(def), "idempotency")
}

func asyncPaymentHandler(w http.ResponseWriter, req *http.Request) {
	id := pathSegments(req.URL.Path)[1]
	switch id {
	case "domain":
		http.Error(w, `{"error":"domain"}`, http.StatusUnprocessableEntity)
	case "missing":
		http.NotFound(w, req)
	case "timeout":
		time.Sleep(100 * time.Millisecond)
		writeJSON(w, http.StatusOK, map[string]interface{}{"id": id})
	default:
		time.Sleep(20 * time.Millisecond)
		writeJSON(w, http.StatusOK, map[string]interface{}{"id": id})
	}
}

func asyncCommand(def Definition, state *AsyncState, init string, input map[string]interface{}) core.Command {
	collection := NewCollection()
	_ = collection.Add(def)
	resolved, _ := collection.ResolveClientOperation(ClientToolConfig{
		RestRef: "payments", Operation: "create_payment",
	})
	return ClientBuilder{
		ToolName: init, Init: init, Operation: resolved, AsyncState: state,
	}.Build(core.Result{Output: mustToolParams(init, input)})
}

func mustToolParams(tool string, input map[string]interface{}) string {
	data, _ := json.Marshal(map[string]interface{}{"tool": tool, "parameters": input})
	return string(data)
}

func asyncDefinition(t *testing.T, baseURL string, client Client) Definition {
	t.Helper()
	client.BaseURL = baseURL
	def := Definition{Version: "v1", Clients: map[string]Client{"payments": client}}
	require.NoError(t, ValidateDefinition(def))
	return def
}

func asyncPaymentClient() Client {
	return Client{Operations: map[string]Operation{"create_payment": asyncPaymentOperation()}}
}

func asyncPaymentOperation() Operation {
	return Operation{
		Method: "POST", Path: "/payments/{order_id}",
		Params:  RequestBinding{Path: map[string]interface{}{"order_id": map[string]interface{}{}}},
		Success: StatusMapping{Status: []int{200}, Signal: "RESTResponded"},
		Failures: []StatusMapping{
			{Status: []int{404}, Signal: "RESTMissing"},
			{Status: []int{422}, Signal: "RESTDomainFailed"},
		},
		SideEffects:   []SideEffect{{Kind: "external_api", State: "payment_created"}},
		Reversibility: Reversibility{Classification: "compensatable", Undo: "cancel_payment"},
		Async: &AsyncClientConfig{
			RequestID: "{{ params.order_id }}", IdempotencyToken: "{{ params.order_id }}",
			Timeout: "30ms", StateRetention: asyncRetentionConsume,
		},
	}
}

func asyncParams(id string) map[string]interface{} {
	return map[string]interface{}{"order_id": id}
}
