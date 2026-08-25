// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package rest loads and validates declarative REST boundary definitions.
package rest

import (
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	restclient "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/client"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/credentials"
)

// DefinitionFile is the top-level YAML document for REST config files.
type DefinitionFile struct {
	Rest Definition `yaml:"rest"`
}

// Definition is the shared REST model used by hand-authored YAML and imports.
type Definition struct {
	Version           string                      `yaml:"version"`
	Clients           map[string]Client           `yaml:"clients,omitempty"`
	Servers           map[string]Server           `yaml:"servers,omitempty"`
	OpenAPI           map[string]OpenAPIImport    `yaml:"openapi,omitempty"`
	Auth              map[string]AuthProfile      `yaml:"auth,omitempty"`
	Limits            map[string]LimitProfile     `yaml:"limits,omitempty"`
	RetryPolicies     map[string]RetryPolicy      `yaml:"retry_policies,omitempty"`
	ResponseMappings  map[string]ResponseMapping  `yaml:"response_mappings,omitempty"`
	DocumentResources map[string]DocumentResource `yaml:"document_resources,omitempty"`
}

// Client-side YAML types live in rest/client and are aliased here so loading,
// validation, and cmd/agent keep compiling until GH-1823.

type Client = restclient.Client
type Resource = restclient.Resource
type Operation = restclient.Operation
type StatusMapping = restclient.StatusMapping
type AuthProfile = restclient.AuthProfile
type LimitProfile = restclient.LimitProfile
type RedirectPolicy = restclient.RedirectPolicy
type NetworkPolicy = restclient.NetworkPolicy
type RetryPolicy = restclient.RetryPolicy
type RequestBinding = restclient.RequestBinding
type ResponseMapping = restclient.ResponseMapping
type AsyncClientConfig = restclient.AsyncClientConfig
type SideEffect = restclient.SideEffect
type Reversibility = restclient.Reversibility

type CredentialResolver = credentials.Resolver
type StaticCredentials = credentials.Static
type EmptyCredentialResolver = credentials.Empty
type EnvironmentCredentials = credentials.Environment

// Server defines one configured inbound REST listener.
type Server struct {
	Address       string                 `yaml:"address"`
	LimitsRef     string                 `yaml:"limits_ref,omitempty"`
	Queue         QueueConfig            `yaml:"queue,omitempty"`
	Endpoints     map[string]Endpoint    `yaml:"endpoints"`
	Shutdown      ShutdownConfig         `yaml:"shutdown,omitempty"`
	LifecycleExit LifecycleExitInjection `yaml:"lifecycle_exit,omitempty"`
}

// LifecycleExitInjection tunes agent-core's per-server synthesis of the
// canonical POST /api/lifecycle/exit lifecycle_control endpoint (GH-1264).
// Injection is on by default so a served agent exposes lifecycle control
// without each profile re-declaring it; Disabled opts a server out (a pure
// fixture, or a monitor server that must carry observability only). AuthRef
// carries an auth profile reference onto the injected endpoint's
// require_auth_ref, and empty keeps parity with today's auth:none control
// servers.
type LifecycleExitInjection struct {
	Disabled bool   `yaml:"disabled,omitempty"`
	AuthRef  string `yaml:"auth_ref,omitempty"`
}

// Endpoint defines one inbound route and binding behavior.
type Endpoint struct {
	OpenAPIOperationID string              `yaml:"openapi_operation_id,omitempty"`
	Method             string              `yaml:"method,omitempty"`
	Path               string              `yaml:"path,omitempty"`
	Binding            string              `yaml:"binding"`
	Signal             string              `yaml:"signal,omitempty"`
	AllowedSignals     []string            `yaml:"allowed_signals,omitempty"`
	SignalField        string              `yaml:"signal_field,omitempty"`
	SignalMapping      map[string]string   `yaml:"signal_mapping,omitempty"`
	LifecycleControl   LifecycleControl    `yaml:"lifecycle_control,omitempty"`
	MonitorView        string              `yaml:"monitor_view,omitempty"`
	Labels             []string            `yaml:"labels,omitempty"`
	Request            RequestBinding      `yaml:"request,omitempty"`
	Response           ResponseMapping     `yaml:"response,omitempty"`
	Queue              QueueConfig         `yaml:"queue,omitempty"`
	MachineRequest     MachineRequest      `yaml:"machine_request,omitempty"`
	SignalSource       SignalSourceBinding `yaml:"signal_source,omitempty"`
	StaticAssets       *StaticAssetsConfig `yaml:"static_assets,omitempty"`
	Redirect           *RedirectConfig     `yaml:"redirect,omitempty"`
	MonitorProxy       *MonitorProxyConfig `yaml:"monitor_proxy,omitempty"`
	Mock               *MockConfig         `yaml:"mock,omitempty"`
}

// MonitorProxyConfig maps agent names to their monitor base URLs for binding
// monitor_proxy, a same-origin reverse proxy. Only these declared upstreams are
// reachable; the caller supplies the agent key and a path suffix, never a host.
type MonitorProxyConfig struct {
	Upstreams map[string]string `yaml:"upstreams"`
}

// RedirectConfig is HTTP redirect response settings for binding redirect.
type RedirectConfig struct {
	Location string `yaml:"location"`
	Status   int    `yaml:"status,omitempty"`
}

// LifecycleControl validates and maps lifecycle control HTTP requests.
type LifecycleControl struct {
	Action         string                 `yaml:"action,omitempty"`
	Signal         string                 `yaml:"signal,omitempty"`
	AllowedSignals []string               `yaml:"allowed_signals,omitempty"`
	TargetSchema   map[string]interface{} `yaml:"target_schema,omitempty"`
	RequireAuthRef string                 `yaml:"require_auth_ref,omitempty"`
}

// StaticAssetsConfig is filesystem-backed static file settings for binding static_assets.
type StaticAssetsConfig struct {
	Root  string `yaml:"root"`
	Index string `yaml:"index,omitempty"`
	SPA   bool   `yaml:"spa,omitempty"`
}

// MachineRequest configures one request-scoped MachineSpec run.
type MachineRequest struct {
	Profile           string                     `yaml:"profile,omitempty"`
	Machine           string                     `yaml:"machine,omitempty"`
	InitialSignal     string                     `yaml:"initial_signal,omitempty"`
	Request           MachineRequestMapping      `yaml:"request,omitempty"`
	Response          MachineRequestResponse     `yaml:"response,omitempty"`
	Timeout           string                     `yaml:"timeout,omitempty"`
	DocumentResources []string                   `yaml:"document_resources,omitempty"`
	MachineSpec       *core.MachineSpec          `yaml:"-"`
	Registry          *core.Registry             `yaml:"-"`
	InitFunc          func(*core.Registry) error `yaml:"-"`
	ToolAction        core.ActionFunc            `yaml:"-"`
	Budget            core.Budget                `yaml:"-"`
	CommandTimeout    string                     `yaml:"-"`
}

// MachineRequestMapping declares which request data seeds the machine.
type MachineRequestMapping struct {
	Body      map[string]string `yaml:"body,omitempty"`
	Query     map[string]string `yaml:"query,omitempty"`
	Path      map[string]string `yaml:"path,omitempty"`
	Headers   map[string]string `yaml:"headers,omitempty"`
	Sensitive []string          `yaml:"sensitive,omitempty"`
}

// MachineRequestResponse maps terminal machine output to HTTP, keyed either by
// the terminal state the run ended in or by the signal that drove it there.
//
// Signal keying alone bounds a machine to as many distinct responses as it has
// distinct terminal signals. Exec words emit only ToolDone, ToolFailed, and
// CommandError, so an exec-driven request machine cannot separate a client
// error from a server error however many terminal states it declares: every
// failure arrives as ToolFailed. Keying by terminal state lifts that bound
// without a bespoke Go word whose only job is to mint a distinguishable signal
// (srd030 R4.3; GH-615).
//
// TerminalStates is consulted first because a state is the more specific fact:
// many signals may reach one state, and one signal may reach many.
type MachineRequestResponse struct {
	TerminalStates  map[string]MachineResponseMapping `yaml:"terminal_states,omitempty"`
	TerminalSignals map[string]MachineResponseMapping `yaml:"terminal_signals,omitempty"`
}

// ResponseMapping resolves the HTTP mapping for one finished run. It returns the
// matched key description so a diagnostic can name what was looked up.
func (r MachineRequestResponse) ResponseMapping(state, signal string) (MachineResponseMapping, string, bool) {
	if mapping, ok := r.TerminalStates[state]; ok {
		return mapping, "terminal state " + state, true
	}
	if mapping, ok := r.TerminalSignals[signal]; ok {
		return mapping, "terminal signal " + signal, true
	}
	return MachineResponseMapping{}, "", false
}

// MachineResponseMapping defines one terminal HTTP response mapping.
type MachineResponseMapping struct {
	Status      int                    `yaml:"status,omitempty"`
	ContentType string                 `yaml:"content_type,omitempty"`
	Headers     map[string]string      `yaml:"headers,omitempty"`
	Body        map[string]string      `yaml:"body,omitempty"`
	Schema      map[string]interface{} `yaml:"schema,omitempty"`
}

// SignalSourceBinding maps validated HTTP data into a trusted SignalEnvelope.
type SignalSourceBinding struct {
	Source             string                       `yaml:"source"`
	DiscriminatorField string                       `yaml:"discriminator_field"`
	SignalMapping      map[string]string            `yaml:"signal_mapping"`
	RunIDField         string                       `yaml:"run_id_field"`
	ExpectedStateField string                       `yaml:"expected_state_field,omitempty"`
	Payload            map[string]string            `yaml:"payload"`
	Sensitive          []string                     `yaml:"sensitive,omitempty"`
	Timeout            string                       `yaml:"timeout"`
	Responses          SignalSourceResponseMappings `yaml:"responses"`
}

// SignalSourceResponseMappings selects HTTP policy without changing admission.
type SignalSourceResponseMappings struct {
	Accepted          SignalSourceResponse `yaml:"accepted"`
	RefusedUndeclared SignalSourceResponse `yaml:"refused_undeclared"`
	RefusedConflict   SignalSourceResponse `yaml:"refused_conflict,omitempty"`
	SourceValidation  SignalSourceResponse `yaml:"source_validation"`
	MachineRunFailed  SignalSourceResponse `yaml:"machine_run_failed"`
}

// SignalSourceResponse controls status and redacted run disclosure.
type SignalSourceResponse struct {
	Status            int  `yaml:"status"`
	IncludeDiagnostic bool `yaml:"include_diagnostic,omitempty"`
	IncludeOutput     bool `yaml:"include_output,omitempty"`
}

// DocumentResource is reserved target-format config for document corpora.
type DocumentResource struct {
	Root                string                       `yaml:"root,omitempty"`
	Include             []string                     `yaml:"include,omitempty"`
	Extensions          []string                     `yaml:"extensions,omitempty"`
	ResponseModes       []string                     `yaml:"response_modes,omitempty"`
	DefaultResponseMode string                       `yaml:"default_response_mode,omitempty"`
	CategoryRules       []DocumentCategoryRule       `yaml:"category_rules,omitempty"`
	MaxBytes            int                          `yaml:"max_bytes,omitempty"`
	Symlinks            string                       `yaml:"symlinks,omitempty"`
	BinaryPolicy        string                       `yaml:"binary_policy,omitempty"`
	Operations          map[string]DocumentOperation `yaml:"operations,omitempty"`
	UI                  DocumentResourceUI           `yaml:"ui,omitempty"`
}

// DocumentCategoryRule maps a path prefix to a document category.
type DocumentCategoryRule struct {
	Prefix   string `yaml:"prefix,omitempty"`
	Category string `yaml:"category,omitempty"`
}

// DocumentOperation is reserved target-format config for document words.
type DocumentOperation struct {
	Type           string            `yaml:"type,omitempty"`
	ResponseMode   string            `yaml:"response_mode,omitempty"`
	SuccessSignal  string            `yaml:"success_signal,omitempty"`
	NotFoundSignal string            `yaml:"not_found_signal,omitempty"`
	DeniedSignal   string            `yaml:"denied_signal,omitempty"`
	Output         map[string]string `yaml:"output,omitempty"`
}

// DocumentResourceUI is reserved human-facing resource presentation config.
type DocumentResourceUI struct {
	Label           string   `yaml:"label,omitempty"`
	SidebarGrouping string   `yaml:"sidebar_grouping,omitempty"`
	Actions         []string `yaml:"actions,omitempty"`
	AssetRef        string   `yaml:"asset_ref,omitempty"`
}

// ShutdownConfig defines graceful server shutdown behavior.
type ShutdownConfig struct {
	Timeout            string `yaml:"timeout,omitempty"`
	DrainPolicy        string `yaml:"drain_policy,omitempty"`
	DrainTimeout       string `yaml:"drain_timeout,omitempty"`
	StopListeners      *bool  `yaml:"stop_listeners,omitempty"`
	QueueOnShutdown    string `yaml:"queue_on_shutdown,omitempty"`
	UnblockAwaitSignal string `yaml:"unblock_await_signal,omitempty"`
}

// OpenAPIImport describes one future OpenAPI source document.
type OpenAPIImport struct {
	Path          string                   `yaml:"path"`
	BaseURL       string                   `yaml:"base_url,omitempty"`
	Expose        []string                 `yaml:"expose,omitempty"`
	Bind          map[string]string        `yaml:"bind,omitempty"`
	SideEffects   map[string][]SideEffect  `yaml:"side_effects,omitempty"`
	Reversibility map[string]Reversibility `yaml:"reversibility,omitempty"`
}

// QueueConfig defines async inbox behavior.
type QueueConfig struct {
	Name         string                 `yaml:"name,omitempty"`
	Capacity     int                    `yaml:"capacity,omitempty"`
	Overflow     string                 `yaml:"overflow,omitempty"`
	Timeout      string                 `yaml:"timeout,omitempty"`
	PayloadShape map[string]interface{} `yaml:"payload_shape,omitempty"`
}
