// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"fmt"
	"net/http"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
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

const defaultPollInterval = 100 * time.Millisecond

func (c clientCmd) awaitAsync() core.Result {
	if c.asyncState == nil {
		return clientOperationError(c.toolName, "async_state_missing", fmt.Errorf("async state is not configured"), c.operation)
	}
	request, err := c.awaitRequest()
	if err != nil {
		return clientOperationError(c.toolName, "async_state_missing", err, c.operation)
	}
	if async := c.operation.Operation.Async; async != nil && async.AwaitOperation != "" {
		return c.awaitByPolling(request, *async)
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

// awaitByPolling drives the configured await_operation as a read poll. It
// resolves the referenced client operation and samples it until the operation
// maps its configured success status (completion), then maps that response to
// the poll operation's success signal. Non-success samples (a not-yet-ready
// status the operation does not map, a transient transport error, or a mapped
// non-success signal) are retried until the async timeout elapses, at which
// point the await reports RESTAwaitTimedOut. This is the poll-until-ready model
// documented for async await_operation (srd028-rest-client-tools R5.5).
func (c clientCmd) awaitByPolling(request *AsyncRequest, async AsyncClientConfig) core.Result {
	if c.definitions == nil {
		return clientOperationError(c.toolName, "await_operation_resolve",
			fmt.Errorf("await_operation %q requires REST definitions", async.AwaitOperation), c.operation)
	}
	pollOp, err := c.definitions.ResolveClientOperation(ClientToolConfig{
		RestRef: c.operation.RestRef, Operation: async.AwaitOperation,
	})
	if err != nil {
		return clientOperationError(c.toolName, "await_operation_resolve", err, c.operation)
	}
	input := pollInput(pollOp, request)
	deadline := time.Now().Add(c.awaitTimeout())
	successSignal := core.Signal(pollOp.Operation.Success.Signal)
	for {
		result := executeClientOnce(c.toolName, pollOp, input, c.credentials)
		if successSignal != "" && result.Signal == successSignal {
			c.asyncState.Consume(request)
			result.CommandName = c.toolName
			return result
		}
		if !time.Now().Add(defaultPollInterval).Before(deadline) {
			return core.Result{
				Signal:      core.Signal("RESTAwaitTimedOut"),
				CommandName: c.toolName,
				Output:      jsonOutput(asyncTimeoutOutput(request)),
			}
		}
		time.Sleep(defaultPollInterval)
	}
}

// pollInput builds the declared runtime input for the await poll operation from
// the submitted async request. It selects only the parameters the poll
// operation declares — preserving the declared-only runtime input contract —
// and backfills the request id under the common resource-id parameter names so
// a read keyed by the created resource resolves its path.
func pollInput(pollOp ClientOperationDefinition, request *AsyncRequest) map[string]interface{} {
	declared := declaredParamNames(pollOp.Operation.Params)
	source := map[string]interface{}{}
	for name, value := range request.SubmittedPayload {
		source[name] = value
	}
	if request.RequestID != "" {
		for _, name := range []string{"id", "request_id", "order_id"} {
			if _, ok := source[name]; !ok {
				source[name] = request.RequestID
			}
		}
	}
	params := map[string]interface{}{}
	for name := range declared {
		if value, ok := source[name]; ok {
			params[name] = value
		}
	}
	return map[string]interface{}{"params": params}
}

// executeClientOnce runs one client operation request and maps the response
// without async accounting. It is the single-shot form the await poll uses to
// sample the referenced read operation.
func executeClientOnce(
	toolName string,
	op ClientOperationDefinition,
	input map[string]interface{},
	creds CredentialResolver,
) core.Result {
	request, err := buildClientRequest(op, input, creds)
	if err != nil {
		return clientOperationError(toolName, requestBuildFailureStage(err), err, op)
	}
	start := time.Now()
	response, err := httpClient(op.Limits).Do(request)
	if err != nil {
		return clientOperationError(toolName, "network_io", redactError(err, op, creds), op)
	}
	defer response.Body.Close()
	result, _ := mapClientResponse(toolName, op, response, 1, time.Since(start))
	return result
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
