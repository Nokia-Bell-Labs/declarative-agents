// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	restclient "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/client"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/credentials"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/redact"
)

// ClientBuilder, CompensationExecutor, and AsyncState live in rest/client and
// are re-exported so factories, lifecycle, and resttest keep compiling.

type ClientBuilder = restclient.ClientBuilder
type CompensationExecutor = restclient.CompensationExecutor
type AsyncState = restclient.AsyncState
type AsyncRequest = restclient.AsyncRequest

// NewAsyncState creates empty async request state.
func NewAsyncState() *AsyncState {
	return restclient.NewAsyncState()
}

func runtimeParams(output string) (map[string]interface{}, error) {
	return restclient.RuntimeParams(output)
}

func selectorValue(selector string, payload map[string]interface{}) interface{} {
	return restclient.SelectorValue(selector, payload)
}

func resolveResultSelector(selector string, source map[string]interface{}) (interface{}, bool) {
	return restclient.ResolveResultSelector(selector, source)
}

func bearerValue(scheme, token string) string {
	return restclient.BearerValue(scheme, token)
}

func validateBodySchema(schema map[string]interface{}, payload map[string]interface{}) error {
	return restclient.ValidateBodySchema(schema, payload)
}

func resolveCredential(resolver credentials.Resolver, ref string) (string, error) {
	return credentials.Resolve(resolver, ref)
}

func redactServerPayload(payload map[string]interface{}, selectors []string) {
	redact.ServerPayload(payload, selectors)
}

func redactMappedOutput(output map[string]interface{}, selectors []string) {
	redact.MappedOutput(output, selectors)
}
