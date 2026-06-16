// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

const responseReadLimit = 1 << 20

var errResponseTooLarge = errors.New("REST response exceeds configured max_response_bytes")

func mapClientResponse(
	commandName string,
	def ClientOperationDefinition,
	response *http.Response,
	attempts int,
	duration time.Duration,
) (core.Result, error) {
	body, err := readResponseBody(response, def.Limits.MaxResponseBytes)
	if err != nil {
		return clientOperationError(commandName, responseFailureStage(err), err, def), err
	}
	payload := decodeResponsePayload(body)
	mapping, signal, err := statusMapping(def, response.StatusCode)
	if err != nil {
		return clientOperationError(commandName, "status_mapping", err, def), err
	}
	responseBytes := len(body)
	responseMap := resolvedResponseMapping(def, mapping)
	if err := validateResponsePayload(responseMap, payload); err != nil {
		return clientOperationError(commandName, "response_mapping", err, def), err
	}
	output := responseOutput(def, mapping, response, payload, attempts)
	redactClientOutput(output, clientRedactionSelectors(def, mapping))
	return core.Result{
		Signal: core.Signal(signal), CommandName: commandName,
		Output:  jsonOutput(output),
		Metrics: clientMetrics(response.StatusCode, attempts, duration, signal, responseBytes),
	}, nil
}

func validateResponsePayload(mapping ResponseMapping, payload map[string]interface{}) error {
	if len(mapping.Schema) == 0 {
		return nil
	}
	if err := validateBodySchema(mapping.Schema, payload); err != nil {
		return fmt.Errorf("response schema: %w", err)
	}
	return nil
}

func readResponseBody(response *http.Response, maxBytes int) ([]byte, error) {
	limit := int64(responseReadLimit)
	if maxBytes > 0 {
		limit = int64(maxBytes)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%w: limit %d", errResponseTooLarge, limit)
	}
	return data, nil
}

func responseFailureStage(err error) string {
	if errors.Is(err, errResponseTooLarge) {
		return "size_limit"
	}
	return "response_mapping"
}

func decodeResponsePayload(body []byte) map[string]interface{} {
	payload := map[string]interface{}{}
	if len(body) == 0 {
		return payload
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		payload["raw"] = string(body)
	}
	return payload
}

func statusMapping(def ClientOperationDefinition, status int) (StatusMapping, string, error) {
	if statusIn(status, def.Operation.Success.Status) {
		return def.Operation.Success, def.Operation.Success.Signal, nil
	}
	for _, mapping := range def.Operation.Failures {
		if statusIn(status, mapping.Status) {
			return mapping, mapping.Signal, nil
		}
	}
	return StatusMapping{}, "", fmt.Errorf("status %d is not mapped", status)
}

func statusIn(status int, allowed []int) bool {
	for _, candidate := range allowed {
		if status == candidate {
			return true
		}
	}
	return false
}

func responseOutput(
	def ClientOperationDefinition,
	mapping StatusMapping,
	response *http.Response,
	payload map[string]interface{},
	attempts int,
) map[string]interface{} {
	responseMap := resolvedResponseMapping(def, mapping)
	return map[string]interface{}{
		"rest_ref":          def.RestRef,
		"resource":          def.Resource,
		"operation":         def.OperationName,
		"status":            response.StatusCode,
		"headers":           headerOutput(response.Header),
		"body":              payload,
		"mapped":            mappedOutput(responseMap.Output, payload),
		"resource_id":       selectorValue(responseMap.ResourceID, payload),
		"request_id":        selectorValue(responseMap.RequestID, payload),
		"retry_count":       attempts - 1,
		"domain_error_code": mapping.DomainErrorCode,
	}
}

func resolvedResponseMapping(def ClientOperationDefinition, mapping StatusMapping) ResponseMapping {
	if mapping.ResponseRef != "" {
		return def.ResponseMappings[mapping.ResponseRef]
	}
	if !emptyResponseMapping(mapping.Response) {
		return mapping.Response
	}
	if def.Operation.ResponseRef != "" {
		return def.ResponseMappings[def.Operation.ResponseRef]
	}
	return def.Operation.Response
}

func emptyResponseMapping(mapping ResponseMapping) bool {
	return len(mapping.Schema) == 0 && len(mapping.Output) == 0 &&
		len(mapping.Redact) == 0 && mapping.ResourceID == "" && mapping.RequestID == ""
}

func mappedOutput(selectors map[string]string, payload map[string]interface{}) map[string]interface{} {
	mapped := map[string]interface{}{}
	for name, selector := range selectors {
		mapped[name] = selectorValue(selector, payload)
	}
	return mapped
}

func selectorValue(selector string, payload map[string]interface{}) interface{} {
	if !strings.HasPrefix(selector, "$.") {
		return nil
	}
	value, ok := payload[strings.TrimPrefix(selector, "$.")]
	if !ok {
		return nil
	}
	return value
}

func headerOutput(headers http.Header) map[string]interface{} {
	output := map[string]interface{}{}
	for name, values := range headers {
		if len(values) == 1 {
			output[strings.ToLower(name)] = values[0]
			continue
		}
		output[strings.ToLower(name)] = values
	}
	return output
}

func clientMetrics(status, attempts int, duration time.Duration, signal string, responseBytes int) *core.ToolMetrics {
	return &core.ToolMetrics{
		Total: 1, Passed: 1,
		Details: map[string]interface{}{
			"status": status, "retry_count": attempts - 1,
			"duration_ms": duration.Milliseconds(), "signal": signal,
			"response_bytes": responseBytes,
		},
	}
}

func clientOperationError(commandName, stage string, err error, def ClientOperationDefinition) core.Result {
	output := map[string]interface{}{
		"failure_stage": stage, "message": err.Error(), "signal": string(core.CommandError),
		"rest_ref": def.RestRef, "resource": def.Resource, "operation": def.OperationName,
	}
	return core.Result{Signal: core.CommandError, CommandName: commandName, Output: jsonOutput(output), Err: err}
}

func redactError(err error, def ClientOperationDefinition, resolver CredentialResolver) error {
	message := err.Error()
	for _, secret := range errorRedactionValues(def.Auth, resolver) {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[REDACTED]")
		}
	}
	return fmt.Errorf("%s", message)
}

func errorRedactionValues(auth AuthProfile, resolver CredentialResolver) []string {
	values := []string{auth.TokenRef, auth.PasswordRef}
	for _, ref := range []string{auth.UsernameRef, auth.PasswordRef, auth.TokenRef} {
		if secret, err := resolveCredential(resolver, ref); err == nil {
			values = append(values, secret)
		}
	}
	return values
}
