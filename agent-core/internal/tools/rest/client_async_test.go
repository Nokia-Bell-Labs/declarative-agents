// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestRESTClient_SendRecordsAsyncRequest(t *testing.T) {
	t.Parallel()

	handlerEntered := make(chan struct{})
	releaseHandler := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		close(handlerEntered)
		<-releaseHandler
		writeJSON(w, http.StatusOK, map[string]interface{}{"id": pathSegments(req.URL.Path)[1]})
	}))
	defer upstream.Close()
	def := asyncDefinition(t, upstream.URL, asyncPaymentClient())
	state := NewAsyncState()

	result := asyncCommand(t, def, state, InitClientSend, asyncParams("slow")).Execute()
	require.Equal(t, core.Signal("RESTAccepted"), result.Signal, result.Output)
	require.Contains(t, result.Output, `"request_id":"slow"`)
	require.Contains(t, result.Output, `"idempotency_token":"slow"`)
	select {
	case <-handlerEntered:
	case <-time.After(time.Second):
		t.Fatal("async handler was not entered")
	}
	close(releaseHandler)

	await := asyncCommand(t, def, state, InitClientAwait, map[string]interface{}{"request_id": "slow"}).Execute()
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
			send := asyncCommand(t, def, state, InitClientSend, asyncParams(tc.id)).Execute()
			require.Equal(t, core.Signal("RESTAccepted"), send.Signal, send.Output)

			await := asyncCommand(t, def, state, InitClientAwait, map[string]interface{}{"request_id": tc.id}).Execute()
			require.Equal(t, tc.signal, await.Signal, await.Output)
		})
	}

	state := NewAsyncState()
	def := asyncDefinition(t, "http://127.0.0.1:1", asyncPaymentClient())
	result := asyncCommand(t, def, state, InitClientAwait, map[string]interface{}{"request_id": "missing"}).Execute()
	require.Equal(t, core.CommandError, result.Signal)
	require.Contains(t, result.Output, "async_state_missing")
}

func TestRESTClient_AsyncCorrelationAndIdempotencyHeader(t *testing.T) {
	t.Parallel()

	requireAsyncCorrelationAndIdempotencyHeader(t)
}

func requireAsyncCorrelationAndIdempotencyHeader(t *testing.T) {
	t.Helper()

	idempotencyKeys := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		select {
		case idempotencyKeys <- req.Header.Get("Idempotency-Key"):
		default:
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"id": "corr"})
	}))
	defer upstream.Close()
	client := asyncPaymentClient()
	op := client.Operations["create_payment"]
	op.Params.BodySchema = bodySchema("correlation")
	op.Async.Correlation = "{{ params.correlation }}"
	client.Operations["create_payment"] = op
	def := asyncDefinition(t, upstream.URL, client)
	state := NewAsyncState()

	send := asyncCommand(t, def, state, InitClientSend, map[string]interface{}{
		"order_id": "corr", "correlation": "payment-corr",
	}).Execute()
	require.Equal(t, core.Signal("RESTAccepted"), send.Signal, send.Output)
	select {
	case idempotencyKey := <-idempotencyKeys:
		require.Equal(t, "corr", idempotencyKey)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for async idempotency header")
	}

	await := asyncCommand(t, def, state, InitClientAwait, map[string]interface{}{"correlation": "payment-corr"}).Execute()
	require.Equal(t, core.Signal("RESTResponded"), await.Signal, await.Output)
}

func TestRESTClient_AsyncRetryPolicyValidation(t *testing.T) {
	t.Parallel()

	requireAsyncRetryPolicyValidation(t)
}

func requireAsyncRetryPolicyValidation(t *testing.T) {
	t.Helper()

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

func TestRESTClient_AwaitOperationReferenceValidation(t *testing.T) {
	t.Parallel()

	requireAwaitOperationUnknownReferenceRejected(t)
	requireAwaitOperationDefinedReferenceAccepted(t)
}

// requireAwaitOperationUnknownReferenceRejected proves await_operation is
// validated against the client's defined operations: a reference to an
// operation that does not exist is rejected at definition time.
func requireAwaitOperationUnknownReferenceRejected(t *testing.T) {
	t.Helper()

	def := asyncDefinition(t, "https://api.example", asyncPaymentClient())
	op := def.Clients["payments"].Operations["create_payment"]
	op.Async.AwaitOperation = "get_payment"
	def.Clients["payments"].Operations["create_payment"] = op

	require.ErrorContains(t, ValidateDefinition(def), "await_operation")
}

// requireAwaitOperationDefinedReferenceAccepted proves an await_operation that
// names a defined client operation passes validation (polling is implemented).
func requireAwaitOperationDefinedReferenceAccepted(t *testing.T) {
	t.Helper()

	client := asyncPaymentClient()
	client.Operations["get_payment"] = Operation{
		Method: "GET", Path: "/payments/{order_id}",
		Params:  RequestBinding{Path: map[string]interface{}{"order_id": map[string]interface{}{}}},
		Success: StatusMapping{Status: []int{200}, Signal: "RESTResponded"},
	}
	op := client.Operations["create_payment"]
	op.Async.AwaitOperation = "get_payment"
	client.Operations["create_payment"] = op
	def := asyncDefinition(t, "https://api.example", client)

	require.NoError(t, ValidateDefinition(def))
}

// TestRESTClient_AwaitOperationPolling exercises the async await_operation
// polling model end to end against a mock upstream: create_payment submits and
// is accepted (202), then await polls the referenced get_payment read operation
// until it reports the resource ready (200 -> RESTResponded), and reports
// RESTAwaitTimedOut when the read never becomes ready within the async timeout.
// This is the behavior the sample rest profile's create/await grammar depends on
// (srd028-rest-client-tools R5.5).
func TestRESTClient_AwaitOperationPolling(t *testing.T) {
	t.Parallel()

	t.Run("polls until ready then responds", func(t *testing.T) {
		var mu sync.Mutex
		getCount := 0
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.Method == http.MethodPost {
				writeJSON(w, http.StatusAccepted, map[string]interface{}{"id": "pay1", "correlation_id": "corr1"})
				return
			}
			mu.Lock()
			getCount++
			n := getCount
			mu.Unlock()
			if n < 2 {
				http.NotFound(w, req)
				return
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"id": "pay1", "state": "settled"})
		}))
		defer upstream.Close()

		collection := pollingCollection(t, upstream.URL, "2s")
		state := NewAsyncState()

		send := pollingCommand(t, collection, state, InitClientSend, map[string]interface{}{"order_id": "pay1"}).Execute()
		require.Equal(t, core.Signal("RESTAccepted"), send.Signal, send.Output)

		await := pollingCommand(t, collection, state, InitClientAwait, map[string]interface{}{"request_id": "pay1"}).Execute()
		require.Equal(t, core.Signal("RESTResponded"), await.Signal, await.Output)
		mu.Lock()
		defer mu.Unlock()
		require.GreaterOrEqual(t, getCount, 2, "await should poll the read operation until ready")
	})

	t.Run("times out when never ready", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.Method == http.MethodPost {
				writeJSON(w, http.StatusAccepted, map[string]interface{}{"id": "pay2", "correlation_id": "corr2"})
				return
			}
			http.NotFound(w, req)
		}))
		defer upstream.Close()

		collection := pollingCollection(t, upstream.URL, "300ms")
		state := NewAsyncState()

		send := pollingCommand(t, collection, state, InitClientSend, map[string]interface{}{"order_id": "pay2"}).Execute()
		require.Equal(t, core.Signal("RESTAccepted"), send.Signal, send.Output)

		await := pollingCommand(t, collection, state, InitClientAwait, map[string]interface{}{"request_id": "pay2"}).Execute()
		require.Equal(t, core.Signal("RESTAwaitTimedOut"), await.Signal, await.Output)
	})
}

// pollingCollection builds a payments client whose create_payment submit awaits
// via a get_payment read poll, and returns it as a resolvable collection so the
// await command can look up the referenced poll operation.
func pollingCollection(t *testing.T, baseURL, timeout string) Collection {
	t.Helper()
	client := Client{
		BaseURL: baseURL,
		Operations: map[string]Operation{
			"create_payment": {
				Method: "POST", Path: "/payments/{order_id}",
				Params:        RequestBinding{Path: map[string]interface{}{"order_id": map[string]interface{}{}}},
				Success:       StatusMapping{Status: []int{202}, Signal: "RESTAccepted"},
				SideEffects:   []SideEffect{{Kind: "external_api", State: "payment_submitted"}},
				Reversibility: Reversibility{Classification: "compensatable", Undo: "cancel_payment"},
				Async: &AsyncClientConfig{
					RequestID: "{{ params.order_id }}", AwaitOperation: "get_payment",
					Timeout: timeout, StateRetention: asyncRetentionConsume,
				},
			},
			"get_payment": {
				Method: "GET", Path: "/payments/{order_id}",
				Params:  RequestBinding{Path: map[string]interface{}{"order_id": map[string]interface{}{}}},
				Success: StatusMapping{Status: []int{200}, Signal: "RESTResponded"},
			},
		},
	}
	def := Definition{Version: "v1", Clients: map[string]Client{"payments": client}}
	require.NoError(t, ValidateDefinition(def))
	collection := NewCollection()
	require.NoError(t, collection.Add(def))
	return collection
}

// pollingCommand resolves create_payment from the collection and builds a
// send/await command wired with the resolver the poll path needs.
func pollingCommand(t *testing.T, collection Collection, state *AsyncState, init string, input map[string]interface{}) core.Command {
	t.Helper()
	resolved, err := collection.ResolveClientOperation(ClientToolConfig{RestRef: "payments", Operation: "create_payment"})
	require.NoError(t, err)
	return ClientBuilder{
		ToolName: init, Init: init, Operation: resolved, Definitions: collection,
		AsyncState: state, Credentials: EmptyCredentialResolver{},
	}.Build(core.Result{Output: mustToolParams(t, init, input)})
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

func asyncCommand(t *testing.T, def Definition, state *AsyncState, init string, input map[string]interface{}) core.Command {
	t.Helper()
	collection := NewCollection()
	require.NoError(t, collection.Add(def))
	resolved, err := collection.ResolveClientOperation(ClientToolConfig{
		RestRef: "payments", Operation: "create_payment",
	})
	require.NoError(t, err)
	return ClientBuilder{
		ToolName: init, Init: init, Operation: resolved, AsyncState: state,
	}.Build(core.Result{Output: mustToolParams(t, init, input)})
}

func mustToolParams(t *testing.T, tool string, input map[string]interface{}) string {
	t.Helper()
	data, err := json.Marshal(map[string]interface{}{"tool": tool, "parameters": input})
	require.NoError(t, err)
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
			Timeout: "100ms", StateRetention: asyncRetentionConsume,
		},
	}
}

func asyncParams(id string) map[string]interface{} {
	return map[string]interface{}{"order_id": id}
}
