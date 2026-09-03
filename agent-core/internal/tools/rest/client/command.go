// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/credentials"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/undo"
)

const restCompensationDescription = "execute the configured REST compensation action"

type networkPolicyError struct{ error }
type networkIOError struct{ error }

func (e networkPolicyError) Unwrap() error { return e.error }
func (e networkIOError) Unwrap() error     { return e.error }

func wrapNetworkPolicyError(err error) error {
	if err != nil {
		return networkPolicyError{error: err}
	}
	return nil
}

// ClientBuilder constructs synchronous REST client commands.
type ClientBuilder struct {
	ToolName    string
	Init        string
	Operation   ClientOperationDefinition
	Definitions ClientOperationResolver
	AsyncState  *AsyncState
	Credentials CredentialResolver
	Metrics     core.MetricConfig
}

// CompensationExecutor executes REST compensation from rollback mementos.
type CompensationExecutor struct {
	Definitions ClientOperationResolver
	Credentials CredentialResolver
}

// Build creates one REST client boundary command.
func (b ClientBuilder) Build(res core.Result) core.Command {
	params, err := runtimeParams(res.Output)
	return &clientCmd{
		toolName: b.ToolName, init: b.Init, operation: b.Operation,
		params: params, asyncState: b.AsyncState, credentials: b.Credentials, buildErr: err,
		metrics: b.Metrics, definitions: b.Definitions,
	}
}

// BuildReverser returns an undo-only command for rollback receipt walks.
func (b ClientBuilder) BuildReverser() core.Command {
	return restCompensationCmd{
		toolName: b.ToolName,
		executor: CompensationExecutor{
			Definitions: b.Definitions,
			Credentials: b.Credentials,
		},
	}
}

type clientCmd struct {
	toolName     string
	init         string
	operation    ClientOperationDefinition
	params       map[string]interface{}
	asyncState   *AsyncState
	credentials  CredentialResolver
	definitions  ClientOperationResolver
	buildErr     error
	recorder     monitor.ToolMetricsRecorder
	metrics      core.MetricConfig
	undoMeta     restUndoMetadata
	commandState core.CommandStateView
	traceCtx     oteltrace.SpanContext
}

// SetCommandState receives the read-only command-state view the engine injects
// before dispatch, so a body_source command_state operation can resolve
// $from(label).path selectors against prior steps (srd028 R13, core
// CommandStateAware).
func (c *clientCmd) SetCommandState(view core.CommandStateView) { c.commandState = view }

var _ core.CommandStateAware = (*clientCmd)(nil)

// SetTraceContext receives the active dispatch span the engine injects before
// dispatch, so outbound requests carry its W3C trace context (srd016 R4, core
// TraceContextAware).
func (c *clientCmd) SetTraceContext(sc oteltrace.SpanContext) { c.traceCtx = sc }

var _ core.TraceContextAware = (*clientCmd)(nil)

type restUndoMetadata struct {
	ResourceID       string
	RequestID        string
	IdempotencyToken string
}

func (c *clientCmd) Name() string { return c.toolName }

var _ core.ContextCommand = (*clientCmd)(nil)

func (c *clientCmd) Execute() core.Result {
	return c.executeContext(context.Background())
}

func (c *clientCmd) ExecuteContext(ctx context.Context) core.Result {
	return c.executeContext(ctx)
}

func (c *clientCmd) executeContext(ctx context.Context) core.Result {
	if c.buildErr != nil {
		return clientOperationError(c.toolName, "schema_validation", c.buildErr, c.operation)
	}
	if c.init == initAwait {
		return c.awaitAsyncContext(ctx)
	}
	request, effective, err := buildClientRequest(
		ctx, c.operation, c.params, c.credentials, c.commandState, c.traceCtx,
	)
	if err != nil {
		return clientOperationError(c.toolName, requestBuildFailureStage(err), err, c.operation)
	}
	c.params = effective
	if c.init == initSend {
		return c.sendAsync(request)
	}
	return c.executeRequest(request)
}

func requestBuildFailureStage(err error) string {
	if credentials.IsResolutionError(err) {
		return "auth_resolution"
	}
	var targetErr targetResolutionError
	if errors.As(err, &targetErr) {
		return "target_resolution"
	}
	return networkFailureStage(err, "request_rendering")
}

func networkFailureStage(err error, fallback string) string {
	var networkErr networkIOError
	if errors.As(err, &networkErr) {
		return "network_io"
	}
	var policyErr networkPolicyError
	if errors.As(err, &policyErr) {
		return "network_policy"
	}
	return fallback
}

func (c *clientCmd) Undo(_ core.Result) core.Result {
	if c.hasRESTCompensation() {
		return undo.BoundaryCompensationUndo(c.toolName, restCompensationDescription)
	}
	return core.NoopUndo(c.toolName)
}

func (c *clientCmd) hasRESTCompensation() bool {
	return c.operation.Operation.Reversibility.Classification == "compensatable" &&
		len(c.operation.Operation.Compensation) > 0
}

func (c *clientCmd) restUndoPayload() undo.BoundaryCompensationPayload {
	return undo.BoundaryCompensationPayload{BoundaryCompensation: undo.BoundaryCompensation{
		Strategy: c.restCompensationStrategy(),
		Reason:   restCompensationDescription,
		Requires: []string{"rest_ref", "operation", "compensation"},
		Data: map[string]interface{}{
			"rest_ref": c.operation.RestRef, "resource": c.operation.Resource,
			"operation": c.operation.OperationName, "parameters": cloneRESTParams(c.params),
			"resource_id": c.restResourceID(), "request_id": c.restRequestID(),
			"idempotency_token": c.restIdempotencyToken(),
			"compensation":      c.operation.Operation.Compensation,
		},
	}}
}

func (c *clientCmd) restCompensationStrategy() string {
	if c.operation.Operation.Reversibility.Undo != "" {
		return c.operation.Operation.Reversibility.Undo
	}
	return "rest_compensation"
}

func (c *clientCmd) restResourceID() string {
	if c.undoMeta.ResourceID != "" {
		return c.undoMeta.ResourceID
	}
	return stringParam(c.params, "id", "number", "resource_id")
}

func (c *clientCmd) restRequestID() string {
	if c.undoMeta.RequestID != "" {
		return c.undoMeta.RequestID
	}
	if c.operation.Operation.Async != nil {
		return asyncValue(c.operation.Operation.Async.RequestID, c.params)
	}
	return stringParam(c.params, "request_id")
}

func (c *clientCmd) restIdempotencyToken() string {
	if c.undoMeta.IdempotencyToken != "" {
		return c.undoMeta.IdempotencyToken
	}
	if c.operation.Operation.Async != nil {
		return asyncValue(c.operation.Operation.Async.IdempotencyToken, c.params)
	}
	return ""
}

func (c *clientCmd) captureRESTUndoMetadata(request *http.Request, result core.Result) {
	c.undoMeta = restUndoMetadata{IdempotencyToken: request.Header.Get("Idempotency-Key")}
	if !c.hasRESTCompensation() {
		return
	}
	output := decodeRESTResultOutput(result.Output)
	c.undoMeta.ResourceID = stringOutputField(output, "resource_id")
	c.undoMeta.RequestID = stringOutputField(output, "request_id")
}

func decodeRESTResultOutput(output string) map[string]interface{} {
	values := map[string]interface{}{}
	_ = json.Unmarshal([]byte(output), &values)
	return values
}

func stringOutputField(output map[string]interface{}, key string) string {
	if value, ok := output[key]; ok {
		return fmt.Sprint(value)
	}
	return ""
}

func (c *clientCmd) executeRequest(request *http.Request) core.Result {
	start := time.Now()
	response, attempts, err := c.doWithRetry(request)
	duration := time.Since(start)
	if err != nil {
		result := clientOperationError(
			c.toolName, networkFailureStage(err, "network_io"),
			redactError(err, c.operation, c.credentials), c.operation,
		)
		if cancellationIsIndeterminate(request, err) {
			output := decodeRESTResultOutput(result.Output)
			output["outcome"] = "indeterminate"
			result.Output = jsonOutput(output)
		}
		return result
	}
	defer func() { _ = response.Body.Close() }()
	result, err := mapClientResponse(c.toolName, c.operation, response, attempts, duration, c.params)
	if err != nil {
		return result
	}
	c.captureRESTUndoMetadata(request, result)
	if c.hasRESTCompensation() {
		result.Receipt = undo.EncodeBoundaryReceipt(c.restUndoPayload())
	}
	c.recordRESTMetrics(request, result)
	return result
}

func cancellationIsIndeterminate(request *http.Request, err error) bool {
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if request.Header.Get("Idempotency-Key") != "" {
		return false
	}
	switch request.Method {
	case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete, http.MethodOptions, http.MethodTrace:
		return false
	default:
		return true
	}
}

// RuntimeParams decodes a prior Result output into REST runtime parameters.
func RuntimeParams(output string) (map[string]interface{}, error) {
	return runtimeParams(output)
}

func runtimeParams(output string) (map[string]interface{}, error) {
	if output == "" {
		return map[string]interface{}{}, nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(output), &raw); err != nil {
		// A prior Result that is not a JSON object carries no runtime
		// parameters. This is the machine seed ("Begin."/"Resume.") or any
		// non-REST word's plain-text output, so a REST word may be the first
		// action in a sentence. Operations that require parameters still fail
		// later, at input-mapping or body rendering, with an operation-specific
		// error instead of an opaque JSON parse failure.
		return map[string]interface{}{}, nil
	}
	if params, ok := raw["parameters"]; ok {
		return decodeRuntimeMap(params)
	}
	return decodeRuntimeMap(json.RawMessage(output))
}

func decodeRuntimeMap(data json.RawMessage) (map[string]interface{}, error) {
	params := map[string]interface{}{}
	if len(data) == 0 || string(data) == "null" {
		return params, nil
	}
	if err := json.Unmarshal(data, &params); err != nil {
		return nil, err
	}
	return params, nil
}
