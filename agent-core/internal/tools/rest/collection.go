// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"context"
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	restclient "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/client"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/credentials"
	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
)

// Collection indexes REST definitions loaded for one profile.
type Collection struct {
	Clients          map[string]restdef.Client
	Servers          map[string]restdef.Server
	Auth             map[string]restdef.AuthProfile
	Limits           map[string]restdef.LimitProfile
	RetryPolicies    map[string]restdef.RetryPolicy
	ResponseMappings map[string]restdef.ResponseMapping
}

// ClientOperationResolver resolves trusted REST client operations.
type ClientOperationResolver = restclient.OperationResolver

// ClientOperationDefinition is a resolved client operation and trusted policy.
type ClientOperationDefinition = restclient.ClientOperationDefinition

// ServerDefinition is a resolved server plus its referenced limit profile.
type ServerDefinition struct {
	Name                 string
	Server               restdef.Server
	Limits               restdef.LimitProfile
	Auth                 map[string]restdef.AuthProfile
	Credentials          credentials.Resolver
	MachineRequestRunner MachineRequestRunner
	SignalSourceRunner   SignalSourceRunner
	Monitor              MonitorState
	RunID                string
}

// MonitorState provides read-only state for monitor REST endpoints.
type MonitorState struct {
	Store            *monitor.Store
	Recorder         monitor.RuntimeRecorder
	Machine          *core.MachineSpec
	DeclaredMachines []core.MachineSpec
	// DeclaredTools is the closure's tool declarations as authored (srd033
	// R9), served by monitor_view declared_tools and never consulted for
	// registration or dispatch.
	DeclaredTools []map[string]interface{}
	Tools            []catalog.ToolDef
	// CommandState backs the opt-in command_state view. It exposes the
	// redaction-cleared declared output of profile-named steps and is nil when no
	// live source is wired (srd033-monitor-rest-api R7).
	CommandState core.CommandStateSource
}

// MachineRequestRunner runs one request-scoped machine.
type MachineRequestRunner interface {
	RunMachineRequest(context.Context, MachineRequestRun) (MachineRequestResult, error)
}

// SignalSourceRunner admits into the host program, never a nested request machine.
type SignalSourceRunner interface {
	RequestSignal(context.Context, core.SignalEnvelope) core.SignalAdmission
}

// MachineRequestRun is the accepted HTTP request visible to a request machine.
type MachineRequestRun struct {
	Server          string                  `json:"server"`
	Route           string                  `json:"route"`
	Method          string                  `json:"method"`
	Path            string                  `json:"path"`
	RequestID       string                  `json:"request_id,omitempty"`
	Payload         map[string]interface{}  `json:"payload,omitempty"`
	Config          restdef.MachineRequest  `json:"-"`
	MonitorRecorder monitor.RuntimeRecorder `json:"-"`
	RunID           string                  `json:"-"`
	ConversationID  string                  `json:"-"`
}

// MachineRequestResult records the short-lived machine outcome.
type MachineRequestResult struct {
	Server         string                 `json:"server,omitempty"`
	Route          string                 `json:"route,omitempty"`
	Machine        string                 `json:"machine,omitempty"`
	TerminalSignal string                 `json:"terminal_signal"`
	Output         map[string]interface{} `json:"output,omitempty"`
	Run            core.RunResult         `json:"run"`
	// TraceID is the connected trace's id (hex), so the response caller can fetch
	// the turn's cross-agent trace waterfall from the trace backend (GH-312).
	TraceID string `json:"trace_id,omitempty"`
}

// NewCollection creates an empty REST definition collection.
func NewCollection() Collection {
	return Collection{
		Clients:          map[string]restdef.Client{},
		Servers:          map[string]restdef.Server{},
		Auth:             map[string]restdef.AuthProfile{},
		Limits:           map[string]restdef.LimitProfile{},
		RetryPolicies:    map[string]restdef.RetryPolicy{},
		ResponseMappings: map[string]restdef.ResponseMapping{},
	}
}

// Add merges a validated REST definition into the collection.
func (c Collection) Add(def restdef.Definition) error {
	for name, profile := range def.Auth {
		if _, exists := c.Auth[name]; exists {
			return fmt.Errorf("duplicate REST auth %q", name)
		}
		c.Auth[name] = profile
	}
	for name, limits := range def.Limits {
		if _, exists := c.Limits[name]; exists {
			return fmt.Errorf("duplicate REST limits %q", name)
		}
		c.Limits[name] = limits
	}
	for name, retry := range def.RetryPolicies {
		if _, exists := c.RetryPolicies[name]; exists {
			return fmt.Errorf("duplicate REST retry policy %q", name)
		}
		c.RetryPolicies[name] = retry
	}
	for name, mapping := range def.ResponseMappings {
		if _, exists := c.ResponseMappings[name]; exists {
			return fmt.Errorf("duplicate REST response mapping %q", name)
		}
		c.ResponseMappings[name] = mapping
	}
	for name, client := range def.Clients {
		if _, exists := c.Clients[name]; exists {
			return fmt.Errorf("duplicate REST client %q", name)
		}
		c.Clients[name] = client
	}
	for name, server := range def.Servers {
		if _, exists := c.Servers[name]; exists {
			return fmt.Errorf("duplicate REST server %q", name)
		}
		c.Servers[name] = server
	}
	return nil
}

// ResolveClientOperation returns a client operation with trusted policy config.
func (c Collection) ResolveClientOperation(cfg ClientToolConfig) (ClientOperationDefinition, error) {
	client, ok := c.Clients[cfg.RestRef]
	if !ok {
		return ClientOperationDefinition{}, fmt.Errorf("REST client %q is not defined", cfg.RestRef)
	}
	operation, err := c.resolveOperation(client, cfg)
	if err != nil {
		return ClientOperationDefinition{}, err
	}
	return ClientOperationDefinition{
		RestRef: cfg.RestRef, Resource: cfg.Resource, OperationName: cfg.Operation,
		Client: client, Operation: operation, Auth: c.Auth[client.AuthRef],
		Limits: c.Limits[client.LimitsRef], Retry: c.RetryPolicies[client.RetryRef],
		ResponseMappings: c.ResponseMappings,
	}, nil
}

func (c Collection) resolveOperation(client restdef.Client, cfg ClientToolConfig) (restdef.Operation, error) {
	if cfg.Resource == "" {
		return operationByName(client.Operations, cfg.Operation, "client "+cfg.RestRef)
	}
	resource, ok := client.Resources[cfg.Resource]
	if !ok {
		return restdef.Operation{}, fmt.Errorf("REST resource %q is not defined on client %q", cfg.Resource, cfg.RestRef)
	}
	operation, err := operationByName(resource.Operations, cfg.Operation, "resource "+cfg.Resource)
	if err != nil {
		return restdef.Operation{}, err
	}
	if operation.Path == "" {
		operation.Path = resource.Path
	}
	return operation, nil
}

// ResolveServer returns a server with the limit profile it references.
func (c Collection) ResolveServer(name string) (ServerDefinition, error) {
	server, ok := c.Servers[name]
	if !ok {
		return ServerDefinition{}, fmt.Errorf("REST server %q is not defined", name)
	}
	return ServerDefinition{
		Name: name, Server: server, Limits: c.Limits[server.LimitsRef], Auth: c.Auth,
	}, nil
}

func operationByName(operations map[string]restdef.Operation, name, owner string) (restdef.Operation, error) {
	operation, ok := operations[name]
	if !ok {
		return restdef.Operation{}, fmt.Errorf("REST operation %q is not defined on %s", name, owner)
	}
	return operation, nil
}
