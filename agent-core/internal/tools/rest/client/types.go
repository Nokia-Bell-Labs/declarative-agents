// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package client

import (
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/credentials"
	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
)

// YAML model types live in rest/definition. Aliases here keep the outbound
// execution path compiling without importing the parent rest package.

type Client = restdef.Client
type Resource = restdef.Resource
type Operation = restdef.Operation
type StatusMapping = restdef.StatusMapping
type AuthProfile = restdef.AuthProfile
type LimitProfile = restdef.LimitProfile
type RedirectPolicy = restdef.RedirectPolicy
type NetworkPolicy = restdef.NetworkPolicy
type RetryPolicy = restdef.RetryPolicy
type RequestBinding = restdef.RequestBinding
type ResponseMapping = restdef.ResponseMapping
type AsyncClientConfig = restdef.AsyncClientConfig
type SideEffect = restdef.SideEffect
type Reversibility = restdef.Reversibility

// OperationResolver resolves trusted REST client operations.
type OperationResolver interface {
	ResolveClientOperation(ClientToolConfig) (ClientOperationDefinition, error)
}

// ClientOperationResolver is the historical name for OperationResolver.
type ClientOperationResolver = OperationResolver

// ClientToolConfig holds REST client ToolDef config.
type ClientToolConfig struct {
	RestRef   string `json:"rest_ref"`
	Resource  string `json:"resource"`
	Operation string `json:"operation"`
}

// ClientOperationDefinition is a resolved client operation and trusted policy.
type ClientOperationDefinition struct {
	RestRef          string
	Resource         string
	OperationName    string
	Client           Client
	Operation        Operation
	Auth             AuthProfile
	Limits           LimitProfile
	Retry            RetryPolicy
	ResponseMappings map[string]ResponseMapping
}

// CredentialResolver is the credentials package resolver.
type CredentialResolver = credentials.Resolver
