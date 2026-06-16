// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"fmt"
	"net/http"
	"time"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

func (c clientCmd) sendAsync(request *http.Request) core.Result {
	async := c.operation.Operation.Async
	requestState := c.asyncRequest(async)
	if c.asyncState == nil {
		c.asyncState = NewAsyncState()
	}
	if err := c.asyncState.Add(requestState); err != nil {
		return clientOperationError(c.toolName, "async_state", err, c.operation)
	}
	go c.completeAsync(request, requestState)
	return core.Result{
		Signal:      core.Signal("RESTAccepted"),
		CommandName: c.toolName,
		Output:      jsonOutput(asyncAcceptedOutput(requestState)),
	}
}

func (c clientCmd) awaitAsync() core.Result {
	if c.asyncState == nil {
		return clientOperationError(c.toolName, "async_state_missing", fmt.Errorf("async state is not configured"), c.operation)
	}
	request, err := c.awaitRequest()
	if err != nil {
		return clientOperationError(c.toolName, "async_state_missing", err, c.operation)
	}
	select {
	case result := <-request.Done:
		c.asyncState.Consume(request)
		result.CommandName = c.toolName
		return result
	case <-time.After(c.awaitTimeout()):
		return core.Result{
			Signal:      core.Signal("RESTAwaitTimedOut"),
			CommandName: c.toolName,
			Output:      jsonOutput(asyncTimeoutOutput(request)),
		}
	}
}

func (c clientCmd) awaitRequest() (*AsyncRequest, error) {
	requestID := stringParam(c.params, "request_id", "id")
	if requestID != "" {
		return c.asyncState.Get(requestID)
	}
	correlation := stringParam(c.params, "correlation")
	if correlation != "" {
		return c.asyncState.GetByCorrelation(correlation)
	}
	return nil, fmt.Errorf("request_id or correlation is required")
}

func (c clientCmd) completeAsync(request *http.Request, state *AsyncRequest) {
	state.Done <- c.executeRequest(request)
}

func (c clientCmd) asyncRequest(async *AsyncClientConfig) *AsyncRequest {
	requestID := asyncValue(async.RequestID, c.params)
	if requestID == "" {
		requestID = c.operation.OperationName + "-" + stringParam(c.params, "id", "order_id", "request_id")
	}
	return &AsyncRequest{
		RequestID:        requestID,
		OperationID:      c.operation.OperationName,
		RestRef:          c.operation.RestRef,
		Resource:         c.operation.Resource,
		IdempotencyToken: asyncValue(async.IdempotencyToken, c.params),
		Correlation:      asyncValue(async.Correlation, c.params),
		SubmittedPayload: payloadSummary(c.params),
		RetentionPolicy:  async.StateRetention,
		Done:             make(chan core.Result, 1),
	}
}

func (c clientCmd) awaitTimeout() time.Duration {
	if c.operation.Operation.Async == nil {
		return defaultAwaitTimeout
	}
	return parseDuration(c.operation.Operation.Async.Timeout, defaultAwaitTimeout)
}

func asyncAcceptedOutput(request *AsyncRequest) map[string]interface{} {
	return map[string]interface{}{
		"request_id":        request.RequestID,
		"operation_id":      request.OperationID,
		"rest_ref":          request.RestRef,
		"resource":          request.Resource,
		"idempotency_token": request.IdempotencyToken,
		"correlation":       request.Correlation,
		"submitted_payload": request.SubmittedPayload,
	}
}

func asyncTimeoutOutput(request *AsyncRequest) map[string]interface{} {
	return map[string]interface{}{
		"request_id": request.RequestID, "operation_id": request.OperationID,
		"rest_ref": request.RestRef, "resource": request.Resource,
		"signal": "RESTAwaitTimedOut",
	}
}

func asyncValue(template string, params map[string]interface{}) string {
	switch {
	case template == "":
		return ""
	case template == "$.id":
		return stringParam(params, "id", "request_id", "order_id")
	case template == "$.request_id":
		return stringParam(params, "request_id", "id")
	case len(bodyParamPattern.FindAllStringSubmatch(template, -1)) > 0:
		return renderAsyncTemplate(template, params)
	default:
		return template
	}
}

func renderAsyncTemplate(template string, params map[string]interface{}) string {
	value, err := renderTemplateString(template, params)
	if err != nil {
		return ""
	}
	return fmt.Sprint(value)
}

func stringParam(params map[string]interface{}, names ...string) string {
	for _, name := range names {
		if value, ok := params[name]; ok {
			return fmt.Sprint(value)
		}
	}
	return ""
}

func payloadSummary(params map[string]interface{}) map[string]interface{} {
	summary := map[string]interface{}{}
	for name, value := range params {
		if forbiddenRuntimeAuthorityFields[name] {
			continue
		}
		summary[name] = value
	}
	return summary
}
