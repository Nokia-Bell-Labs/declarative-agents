// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/monitor"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

// ClientBuilder constructs synchronous REST client commands.
type ClientBuilder struct {
	ToolName    string
	Init        string
	Operation   ClientOperationDefinition
	AsyncState  *AsyncState
	Credentials CredentialResolver
	Metrics     core.MetricConfig
}

// Build creates one REST client boundary command.
func (b ClientBuilder) Build(res core.Result) core.Command {
	params, err := runtimeParams(res.Output)
	return &clientCmd{
		toolName: b.ToolName, init: b.Init, operation: b.Operation,
		params: params, asyncState: b.AsyncState, credentials: b.Credentials, buildErr: err,
		metrics: b.Metrics,
	}
}

type clientCmd struct {
	toolName    string
	init        string
	operation   ClientOperationDefinition
	params      map[string]interface{}
	asyncState  *AsyncState
	credentials CredentialResolver
	buildErr    error
	recorder    monitor.ToolMetricsRecorder
	metrics     core.MetricConfig
}

func (c clientCmd) Name() string { return c.toolName }

func (c clientCmd) Execute() core.Result {
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

func (c clientCmd) Undo() core.Result {
	return core.NoopUndo(c.toolName)
}

func (c clientCmd) executeRequest(request *http.Request) core.Result {
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
