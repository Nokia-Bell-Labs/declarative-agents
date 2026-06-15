// Copyright (c) 2026 Nokia. All rights reserved.

// Package rest loads and validates declarative REST boundary definitions.
package rest

import "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"

// DefinitionFile is the top-level YAML document for REST config files.
type DefinitionFile struct {
	Rest Definition `yaml:"rest"`
}

// Definition is the shared REST model used by hand-authored YAML and imports.
type Definition struct {
	Version          string                     `yaml:"version"`
	Clients          map[string]Client          `yaml:"clients,omitempty"`
	Servers          map[string]Server          `yaml:"servers,omitempty"`
	OpenAPI          map[string]OpenAPIImport   `yaml:"openapi,omitempty"`
	Auth             map[string]AuthProfile     `yaml:"auth,omitempty"`
	Limits           map[string]LimitProfile    `yaml:"limits,omitempty"`
	RetryPolicies    map[string]RetryPolicy     `yaml:"retry_policies,omitempty"`
	ResponseMappings map[string]ResponseMapping `yaml:"response_mappings,omitempty"`
}

// Client defines one configured outbound REST authority.
type Client struct {
	BaseURL    string               `yaml:"base_url,omitempty"`
	AuthRef    string               `yaml:"auth_ref,omitempty"`
	LimitsRef  string               `yaml:"limits_ref,omitempty"`
	RetryRef   string               `yaml:"retry_ref,omitempty"`
	Resources  map[string]Resource  `yaml:"resources,omitempty"`
	Operations map[string]Operation `yaml:"operations,omitempty"`
}

// Resource groups resource-shaped REST operations.
type Resource struct {
	Path         string               `yaml:"path"`
	Operations   map[string]Operation `yaml:"operations"`
	IDField      string               `yaml:"id_field,omitempty"`
	VersionField string               `yaml:"version_field,omitempty"`
}

// Operation defines one outbound REST operation.
type Operation struct {
	OpenAPIOperationID string                 `yaml:"openapi_operation_id,omitempty"`
	Method             string                 `yaml:"method,omitempty"`
	Path               string                 `yaml:"path,omitempty"`
	Params             RequestBinding         `yaml:"params,omitempty"`
	Body               map[string]interface{} `yaml:"body,omitempty"`
	Success            StatusMapping          `yaml:"success"`
	Failures           []StatusMapping        `yaml:"failures,omitempty"`
	ResponseRef        string                 `yaml:"response_ref,omitempty"`
	Response           ResponseMapping        `yaml:"response,omitempty"`
	SideEffects        []SideEffect           `yaml:"side_effects,omitempty"`
	Reversibility      Reversibility          `yaml:"reversibility,omitempty"`
	Compensation       map[string]interface{} `yaml:"compensation,omitempty"`
	Async              *AsyncClientConfig     `yaml:"async,omitempty"`
}

// StatusMapping maps HTTP statuses to grammar signals and response shaping.
type StatusMapping struct {
	Status          []int           `yaml:"status"`
	Signal          string          `yaml:"signal"`
	DomainErrorCode string          `yaml:"domain_error_code,omitempty"`
	ResponseRef     string          `yaml:"response_ref,omitempty"`
	Response        ResponseMapping `yaml:"response,omitempty"`
}

// AuthProfile defines credential references, never inline secret values.
type AuthProfile struct {
	Type        string `yaml:"type"`
	UsernameRef string `yaml:"username_ref,omitempty"`
	PasswordRef string `yaml:"password_ref,omitempty"`
	TokenRef    string `yaml:"token_ref,omitempty"`
	Header      string `yaml:"header,omitempty"`
	Query       string `yaml:"query,omitempty"`
	Scheme      string `yaml:"scheme,omitempty"`
}

// CredentialResolver resolves trusted runtime credentials by reference.
type CredentialResolver interface {
	ResolveCredential(ref string) (string, error)
}

// StaticCredentials resolves credentials from an in-memory trusted map.
type StaticCredentials map[string]string

// EmptyCredentialResolver resolves no credential references.
type EmptyCredentialResolver struct{}

// LimitProfile defines timeout, size, redirect, and network limits.
type LimitProfile struct {
	Timeout          string         `yaml:"timeout,omitempty"`
	ConnectTimeout   string         `yaml:"connect_timeout,omitempty"`
	ReadTimeout      string         `yaml:"read_timeout,omitempty"`
	MaxRequestBytes  int            `yaml:"max_request_bytes,omitempty"`
	MaxResponseBytes int            `yaml:"max_response_bytes,omitempty"`
	MaxHeaderBytes   int            `yaml:"max_header_bytes,omitempty"`
	Redirect         RedirectPolicy `yaml:"redirect,omitempty"`
	Network          NetworkPolicy  `yaml:"network,omitempty"`
}

// RedirectPolicy controls outbound redirect behavior.
type RedirectPolicy struct {
	Mode         string   `yaml:"mode"`
	AllowHosts   []string `yaml:"allow_hosts,omitempty"`
	MaxRedirects int      `yaml:"max_redirects,omitempty"`
}

// NetworkPolicy defines configured destination or listener authority.
type NetworkPolicy struct {
	Schemes             []string `yaml:"schemes,omitempty"`
	Hosts               []string `yaml:"hosts,omitempty"`
	CIDRs               []string `yaml:"cidrs,omitempty"`
	Ports               []int    `yaml:"ports,omitempty"`
	AllowPublicListener bool     `yaml:"allow_public_listener,omitempty"`
}

// RetryPolicy defines outbound retry behavior.
type RetryPolicy struct {
	Attempts           int    `yaml:"attempts"`
	Backoff            string `yaml:"backoff,omitempty"`
	InitialDelay       string `yaml:"initial_delay,omitempty"`
	MaxDelay           string `yaml:"max_delay,omitempty"`
	RetryStatus        []int  `yaml:"retry_status,omitempty"`
	RetryNetworkErrors bool   `yaml:"retry_network_errors,omitempty"`
	RequireIdempotency bool   `yaml:"require_idempotency,omitempty"`
}

// Server defines one configured inbound REST listener.
type Server struct {
	Address   string              `yaml:"address"`
	LimitsRef string              `yaml:"limits_ref,omitempty"`
	Queue     QueueConfig         `yaml:"queue,omitempty"`
	Endpoints map[string]Endpoint `yaml:"endpoints"`
	Shutdown  ShutdownConfig      `yaml:"shutdown,omitempty"`
}

// Endpoint defines one inbound route and binding behavior.
type Endpoint struct {
	OpenAPIOperationID string          `yaml:"openapi_operation_id,omitempty"`
	Method             string          `yaml:"method,omitempty"`
	Path               string          `yaml:"path,omitempty"`
	Binding            string          `yaml:"binding"`
	Signal             string          `yaml:"signal,omitempty"`
	AllowedSignals     []string        `yaml:"allowed_signals,omitempty"`
	Request            RequestBinding  `yaml:"request,omitempty"`
	Response           ResponseMapping `yaml:"response,omitempty"`
	Queue              QueueConfig     `yaml:"queue,omitempty"`
	MachineRequest     MachineRequest  `yaml:"machine_request,omitempty"`
}

// MachineRequest configures one request-scoped MachineSpec run.
type MachineRequest struct {
	Profile        string                     `yaml:"profile,omitempty"`
	Machine        string                     `yaml:"machine,omitempty"`
	Request        MachineRequestMapping      `yaml:"request,omitempty"`
	Response       MachineRequestResponse     `yaml:"response,omitempty"`
	Timeout        string                     `yaml:"timeout,omitempty"`
	MachineSpec    *core.MachineSpec          `yaml:"-"`
	Registry       *core.Registry             `yaml:"-"`
	InitFunc       func(*core.Registry) error `yaml:"-"`
	ToolAction     core.ActionFunc            `yaml:"-"`
	Budget         core.Budget                `yaml:"-"`
	CommandTimeout string                     `yaml:"-"`
}

// MachineRequestMapping declares which request data seeds the machine.
type MachineRequestMapping struct {
	Body     map[string]string `yaml:"body,omitempty"`
	Query    map[string]string `yaml:"query,omitempty"`
	Path     map[string]string `yaml:"path,omitempty"`
	Headers  map[string]string `yaml:"headers,omitempty"`
	Metadata []string          `yaml:"metadata,omitempty"`
}

// MachineRequestResponse maps terminal machine output to HTTP.
type MachineRequestResponse struct {
	TerminalSignals map[string]MachineResponseMapping `yaml:"terminal_signals,omitempty"`
}

// MachineResponseMapping defines one terminal HTTP response mapping.
type MachineResponseMapping struct {
	Status      int               `yaml:"status,omitempty"`
	ContentType string            `yaml:"content_type,omitempty"`
	Headers     map[string]string `yaml:"headers,omitempty"`
	Body        map[string]string `yaml:"body,omitempty"`
}

// ShutdownConfig defines graceful server shutdown behavior.
type ShutdownConfig struct {
	Timeout            string `yaml:"timeout,omitempty"`
	DrainPolicy        string `yaml:"drain_policy,omitempty"`
	DrainTimeout       string `yaml:"drain_timeout,omitempty"`
	StopListeners      bool   `yaml:"stop_listeners,omitempty"`
	QueueOnShutdown    string `yaml:"queue_on_shutdown,omitempty"`
	UnblockAwaitSignal string `yaml:"unblock_await_signal,omitempty"`
}

// RequestBinding declares runtime fields accepted by an operation or endpoint.
type RequestBinding struct {
	Path       map[string]interface{} `yaml:"path,omitempty"`
	Query      map[string]interface{} `yaml:"query,omitempty"`
	Headers    map[string]interface{} `yaml:"headers,omitempty"`
	BodySchema map[string]interface{} `yaml:"body_schema,omitempty"`
	BodySource string                 `yaml:"body_source,omitempty"`
}

// ResponseMapping maps HTTP data into Result output.
type ResponseMapping struct {
	Schema     map[string]interface{} `yaml:"schema,omitempty"`
	Output     map[string]string      `yaml:"output,omitempty"`
	Redact     []string               `yaml:"redact,omitempty"`
	ResourceID string                 `yaml:"resource_id,omitempty"`
	RequestID  string                 `yaml:"request_id,omitempty"`
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

// AsyncClientConfig enables send and await behavior for an operation.
type AsyncClientConfig struct {
	RequestID        string `yaml:"request_id"`
	Correlation      string `yaml:"correlation,omitempty"`
	IdempotencyToken string `yaml:"idempotency_token,omitempty"`
	AwaitOperation   string `yaml:"await_operation,omitempty"`
	Timeout          string `yaml:"timeout,omitempty"`
	StateRetention   string `yaml:"state_retention,omitempty"`
}

// QueueConfig defines async inbox behavior.
type QueueConfig struct {
	Name         string                 `yaml:"name,omitempty"`
	Capacity     int                    `yaml:"capacity,omitempty"`
	Overflow     string                 `yaml:"overflow,omitempty"`
	Timeout      string                 `yaml:"timeout,omitempty"`
	PayloadShape map[string]interface{} `yaml:"payload_shape,omitempty"`
}

// SideEffect declares an observable REST boundary effect.
type SideEffect struct {
	Kind   string `yaml:"kind,omitempty"`
	Target string `yaml:"target,omitempty"`
	State  string `yaml:"state,omitempty"`
}

// Reversibility classifies operation compensation behavior.
type Reversibility struct {
	Classification       string `yaml:"classification,omitempty"`
	Undo                 string `yaml:"undo,omitempty"`
	RequiresConfirmation bool   `yaml:"requires_confirmation,omitempty"`
}
