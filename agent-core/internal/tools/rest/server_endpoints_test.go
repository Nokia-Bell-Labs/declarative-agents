// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestRESTServer_AwaitInboundSignals(t *testing.T) {
	t.Parallel()

	state, baseURL := launchRESTServer(t, controlServer(), LimitProfile{})
	defer stopRESTServer(t, state, "control")

	postStatus(t, baseURL+"/approve/123", `{}`, http.StatusAccepted)
	requireAwaitSignal(t, state, "control", "Approved")

	postStatus(t, baseURL+"/domain?signal=DomainEventReceived", `{}`, http.StatusAccepted)
	requireAwaitSignal(t, state, "control", "DomainEventReceived")

	postStatus(t, baseURL+"/domain?signal=Hidden", `{}`, http.StatusBadRequest)
	requireAwaitSignal(t, state, "control", "AwaitTimedOut")
}

func TestRESTServer_LifecycleControlEnqueuesSignals(t *testing.T) {
	t.Parallel()

	requireLifecycleControlEnqueuesSignal(t)
}

func TestRESTServer_RejectsUndeclaredQueryAndHeader(t *testing.T) {
	t.Parallel()

	state, baseURL := launchRESTServer(t, validationServer(), LimitProfile{MaxRequestBytes: 128})
	defer stopRESTServer(t, state, "validation")

	postStatus(t, baseURL+"/approve/1?unexpected=value", `{}`, http.StatusBadRequest)
	requestStatusWithHeaders(t, http.MethodPost, baseURL+"/approve/1", `{}`, map[string]string{
		"X-Undeclared-Secret": "secret-value",
	}, http.StatusBadRequest)
	postStatus(t, baseURL+"/approve/abc", `{}`, http.StatusBadRequest)
	requestStatusWithHeaders(t, http.MethodPost, baseURL+"/approve/1", `{}`, map[string]string{
		"X-Approval-Token": "wrong-type",
	}, http.StatusBadRequest)

	requireAwaitSignal(t, state, "validation", "AwaitTimedOut")
}

func TestRESTServer_RedactsAwaitAndStreamOutput(t *testing.T) {
	t.Parallel()

	state, baseURL := launchRESTServer(t, redactionServer(), LimitProfile{})
	defer stopRESTServer(t, state, "redaction")

	requestStatusWithHeaders(t, http.MethodPost, baseURL+"/approve/123?token=query-secret",
		`{"secret":"body-secret"}`, map[string]string{"Authorization": "header-secret"}, http.StatusAccepted)
	await := awaitCommand(state, "redaction").Execute().Output
	require.NotContains(t, await, "query-secret")
	require.NotContains(t, await, "header-secret")
	require.NotContains(t, await, "body-secret")
	require.Contains(t, await, "[REDACTED]")

	requestStatusWithHeaders(t, http.MethodPost, baseURL+"/approve/456?token=query-secret",
		`{"secret":"body-secret"}`, map[string]string{"Authorization": "header-secret"}, http.StatusAccepted)
	stream := requestBody(t, http.MethodGet, baseURL+"/events", "", http.StatusOK)
	require.NotContains(t, stream, "query-secret")
	require.NotContains(t, stream, "header-secret")
	require.NotContains(t, stream, "body-secret")
	require.Contains(t, stream, "[REDACTED]")
}

func TestRESTServer_RedactsHandlerResponses(t *testing.T) {
	t.Parallel()

	state, baseURL := launchRESTServer(t, redactionServer(), LimitProfile{})
	defer stopRESTServer(t, state, "redaction")

	result := postJSON(t, baseURL+"/handle-secret", `{"secret":"body-secret"}`, http.StatusOK)
	require.Equal(t, "[REDACTED]", result["secret"])
}

func TestRESTServer_RequestValidationFailures(t *testing.T) {
	t.Parallel()

	state, baseURL := launchRESTServer(t, validationServer(), LimitProfile{MaxRequestBytes: 12})
	defer stopRESTServer(t, state, "validation")

	postStatus(t, baseURL+"/approve/1", `{}`, http.StatusAccepted)
	postStatus(t, baseURL+"/approve/2", `{}`, http.StatusTooManyRequests)
	requestStatus(t, http.MethodGet, baseURL+"/approve/3", "", http.StatusMethodNotAllowed)
	postStatus(t, baseURL+"/typed", `{"name": 42}`, http.StatusBadRequest)
	postStatus(t, baseURL+"/typed", `{"name":"too large"}`, http.StatusRequestEntityTooLarge)
	postStatus(t, baseURL+"/handler", `{}`, http.StatusInternalServerError)

	requireAwaitSignal(t, state, "validation", "Approved")
	requireAwaitSignal(t, state, "validation", "AwaitTimedOut")
}

func TestRESTServer_InvokeHandlerBindings(t *testing.T) {
	t.Parallel()

	state, baseURL := launchRESTServer(t, handlerServer(), LimitProfile{})
	defer stopRESTServer(t, state, "handler")

	result := postJSON(t, baseURL+"/handle", `{"name":"alice"}`, http.StatusOK)
	require.Equal(t, true, result["handled"])
	require.Equal(t, "alice", result["name"])

	postStatus(t, baseURL+"/handle-signal", `{}`, http.StatusOK)
	requireAwaitSignal(t, state, "handler", "Handled")
}

func TestRESTServer_StreamEvents(t *testing.T) {
	t.Parallel()

	state, baseURL := launchRESTServer(t, streamServer(), LimitProfile{})
	defer stopRESTServer(t, state, "stream")

	postStatus(t, baseURL+"/approve/123", `{}`, http.StatusAccepted)
	body := requestBody(t, http.MethodGet, baseURL+"/events", "", http.StatusOK)
	require.Contains(t, body, "event: message")
	require.Contains(t, body, `"signal":"Approved"`)
	require.Contains(t, body, `"route":"approve"`)
}
