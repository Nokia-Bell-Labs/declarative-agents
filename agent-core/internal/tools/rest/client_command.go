// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/undo"
)

const restCompensationDescription = "execute the configured REST compensation action"

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
	toolName    string
	init        string
	operation   ClientOperationDefinition
	params      map[string]interface{}
	asyncState  *AsyncState
	credentials CredentialResolver
	definitions ClientOperationResolver
	buildErr    error
	recorder    monitor.ToolMetricsRecorder
	metrics     core.MetricConfig
	undoMeta    restUndoMetadata
}

type restUndoMetadata struct {
	ResourceID       string
	RequestID        string
	IdempotencyToken string
}

type restCompensationCmd struct {
	toolName string
	executor CompensationExecutor
}

func (c *clientCmd) Name() string { return c.toolName }

func (c restCompensationCmd) Name() string { return c.toolName }

func (c restCompensationCmd) Execute() core.Result {
	return restCompensationError(c.toolName, "compensation_execute", fmt.Errorf("REST compensation commands are undo-only"))
}

func (c restCompensationCmd) Undo(prior core.Result) core.Result {
	commandName := prior.CommandName
	if commandName == "" {
		commandName = c.toolName
	}
	return c.executor.CompensateFromReceipt(context.Background(), commandName, prior.Receipt)
}

func (c *clientCmd) Execute() core.Result {
	if c.buildErr != nil {
		return clientOperationError(c.toolName, "schema_validation", c.buildErr, c.operation)
	}
	if c.init == InitClientAwait {
		return c.awaitAsync()
	}
	request, err := buildClientRequest(c.operation, c.params, c.credentials)
	if err != nil {
		return clientOperationError(c.toolName, requestBuildFailureStage(err), err, c.operation)
	}
	if c.init == InitClientSend {
		return c.sendAsync(request)
	}
	return c.executeRequest(request)
}

func requestBuildFailureStage(err error) string {
	if isCredentialResolutionError(err) {
		return "auth_resolution"
	}
	return "request_rendering"
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
		Strategy:         c.restCompensationStrategy(),
		Reason:           restCompensationDescription,
		Requires:         []string{"rest_ref", "operation", "compensation"},
		RestRef:          c.operation.RestRef,
		Resource:         c.operation.Resource,
		Operation:        c.operation.OperationName,
		Parameters:       cloneRESTParams(c.params),
		ResourceID:       c.restResourceID(),
		RequestID:        c.restRequestID(),
		IdempotencyToken: c.restIdempotencyToken(),
		Compensation:     c.operation.Operation.Compensation,
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

// CompensateFromReceipt executes the REST compensation described by an opaque
// receipt captured in Result.Receipt during Execute. This is the receipt-driven
// entry point used by the reverse receipt walk (srd035-checkpoint-port R3; #44 R3).
func (e CompensationExecutor) CompensateFromReceipt(_ context.Context, commandName, receipt string) core.Result {
	compensation, ok, err := undo.DecodeBoundaryReceipt(receipt)
	if err != nil {
		return restCompensationError(commandName, "compensation_decode", err)
	}
	if !ok {
		return core.NoopUndo(commandName)
	}
	return e.runCompensation(commandName, compensation)
}

func (e CompensationExecutor) runCompensation(commandName string, compensation undo.BoundaryCompensation) core.Result {
	operation, err := e.resolveCompensationOperation(compensation)
	if err != nil {
		return restCompensationError(commandName, "compensation_lookup", err)
	}
	cmd := ClientBuilder{
		ToolName:    compensationToolName(commandName),
		Init:        InitClientInvoke,
		Operation:   operation,
		Credentials: e.Credentials,
	}.Build(core.Result{Output: jsonOutput(compensationRuntimeParams(compensation, operation.Operation.Params))})
	result := cmd.Execute()
	if result.Signal == core.CommandError {
		return result
	}
	result.CommandName = commandName
	return result
}

func (e CompensationExecutor) resolveCompensationOperation(
	compensation undo.BoundaryCompensation,
) (ClientOperationDefinition, error) {
	if e.Definitions == nil {
		return ClientOperationDefinition{}, fmt.Errorf("REST compensation definitions are not configured")
	}
	operationName, ok := compensation.Compensation["operation"].(string)
	if !ok || operationName == "" {
		return ClientOperationDefinition{}, fmt.Errorf("REST compensation operation is not configured")
	}
	return e.Definitions.ResolveClientOperation(ClientToolConfig{
		RestRef: compensation.RestRef, Resource: compensation.Resource, Operation: operationName,
	})
}

func compensationToolName(commandName string) string {
	if commandName == "" {
		return "rest_compensation"
	}
	return commandName + "_compensation"
}

func compensationRuntimeParams(compensation undo.BoundaryCompensation, binding RequestBinding) map[string]interface{} {
	params := map[string]interface{}{}
	declared := declaredParamNames(binding)
	copyCompensationParams(params, compensation.Parameters)
	copyCompensationParams(params, compensation.Compensation["parameters"])
	setCompensationParam(params, declared, "resource_id", compensation.ResourceID)
	setCompensationParam(params, declared, "id", compensation.ResourceID)
	setCompensationParam(params, declared, "number", compensation.ResourceID)
	copyCompensationParam(params, declared, "request_id", compensation.RequestID)
	copyCompensationParam(params, declared, "idempotency_token", compensation.IdempotencyToken)
	dropUndeclaredCompensationParams(params, declared)
	return map[string]interface{}{"parameters": params}
}

func cloneRESTParams(params map[string]interface{}) map[string]interface{} {
	if params == nil {
		return nil
	}
	clone := make(map[string]interface{}, len(params))
	for name, value := range params {
		clone[name] = value
	}
	return clone
}

func copyCompensationParams(params map[string]interface{}, value interface{}) {
	configured, ok := value.(map[string]interface{})
	if !ok {
		return
	}
	for name, param := range configured {
		params[name] = param
	}
}

func copyCompensationParam(params map[string]interface{}, declared map[string]bool, name, value string) {
	if value == "" || !declared[name] {
		return
	}
	if _, exists := params[name]; !exists {
		params[name] = value
	}
}

func setCompensationParam(params map[string]interface{}, declared map[string]bool, name, value string) {
	if value == "" || !declared[name] {
		return
	}
	params[name] = value
}

func dropUndeclaredCompensationParams(params map[string]interface{}, declared map[string]bool) {
	for name := range params {
		if !declared[name] {
			delete(params, name)
		}
	}
}

func restCompensationError(commandName, stage string, err error) core.Result {
	output := map[string]interface{}{
		"failure_stage": stage,
		"message":       err.Error(),
		"signal":        string(core.CommandError),
	}
	return core.Result{Signal: core.CommandError, CommandName: commandName, Output: jsonOutput(output), Err: err}
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
		return clientOperationError(c.toolName, "network_io", redactError(err, c.operation, c.credentials), c.operation)
	}
	defer response.Body.Close()
	result, err := mapClientResponse(c.toolName, c.operation, response, attempts, duration)
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

func (c clientCmd) doWithRetry(request *http.Request) (*http.Response, int, error) {
	client := httpClient(c.operation.Limits)
	attempts := retryAttempts(c.operation.Retry)
	for attempt := 1; attempt <= attempts; attempt++ {
		response, err := client.Do(cloneRequest(request))
		if shouldReturnResponse(response, err, attempt, attempts, c.operation.Retry) {
			return response, attempt, err
		}
		closeResponse(response)
		time.Sleep(parseDuration(c.operation.Retry.InitialDelay, 0))
	}
	return nil, attempts, fmt.Errorf("REST request failed after %d attempts", attempts)
}

func httpClient(limits LimitProfile) *http.Client {
	client := &http.Client{Timeout: parseDuration(limits.Timeout, 0)}
	client.CheckRedirect = redirectPolicy(limits)
	return client
}

func redirectPolicy(limits LimitProfile) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		policy := limits.Redirect
		if policy.Mode == redirectNone || policy.Mode == "" {
			return http.ErrUseLastResponse
		}
		if err := validateNetwork(req.URL, limits.Network); err != nil {
			return err
		}
		if policy.Mode == redirectSameHost && len(via) > 0 && req.URL.Host != via[0].URL.Host {
			return http.ErrUseLastResponse
		}
		if policy.Mode == redirectAllowlist && !stringIn(req.URL.Hostname(), policy.AllowHosts) {
			return fmt.Errorf("redirect host %q is not allowed", req.URL.Hostname())
		}
		if policy.MaxRedirects > 0 && len(via) >= policy.MaxRedirects {
			return http.ErrUseLastResponse
		}
		return nil
	}
}

func retryAttempts(policy RetryPolicy) int {
	if policy.Attempts > 0 {
		return policy.Attempts
	}
	return 1
}

func shouldReturnResponse(response *http.Response, err error, attempt, max int, retry RetryPolicy) bool {
	if attempt >= max {
		return true
	}
	if err != nil {
		return !retry.RetryNetworkErrors
	}
	return !statusIn(response.StatusCode, retry.RetryStatus)
}

func closeResponse(response *http.Response) {
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
}

func cloneRequest(request *http.Request) *http.Request {
	clone := request.Clone(request.Context())
	if request.GetBody != nil {
		body, _ := request.GetBody()
		clone.Body = body
	}
	return clone
}

func runtimeParams(output string) (map[string]interface{}, error) {
	if output == "" {
		return map[string]interface{}{}, nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(output), &raw); err != nil {
		return nil, err
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
