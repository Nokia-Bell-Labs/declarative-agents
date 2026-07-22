// Copyright (c) 2026 Nokia. All rights reserved.

// Package rest loads and validates declarative REST boundary definitions.
package rest

import "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"

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
	OpenAPIOperationID string              `yaml:"openapi_operation_id,omitempty"`
	Method             string              `yaml:"method,omitempty"`
	Path               string              `yaml:"path,omitempty"`
	Binding            string              `yaml:"binding"`
	Signal             string              `yaml:"signal,omitempty"`
	AllowedSignals     []string            `yaml:"allowed_signals,omitempty"`
	LifecycleControl   LifecycleControl    `yaml:"lifecycle_control,omitempty"`
	MonitorView        string              `yaml:"monitor_view,omitempty"`
	Request            RequestBinding      `yaml:"request,omitempty"`
	Response           ResponseMapping     `yaml:"response,omitempty"`
	Queue              QueueConfig         `yaml:"queue,omitempty"`
	MachineRequest     MachineRequest      `yaml:"machine_request,omitempty"`
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
	Body    map[string]string `yaml:"body,omitempty"`
	Query   map[string]string `yaml:"query,omitempty"`
	Path    map[string]string `yaml:"path,omitempty"`
	Headers map[string]string `yaml:"headers,omitempty"`
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

// RequestBinding declares runtime fields accepted by an operation or endpoint.
type RequestBinding struct {
	Path       map[string]interface{} `yaml:"path,omitempty"`
	Query      map[string]interface{} `yaml:"query,omitempty"`
	Headers    map[string]interface{} `yaml:"headers,omitempty"`
	BodySchema map[string]interface{} `yaml:"body_schema,omitempty"`
	BodySource string                 `yaml:"body_source,omitempty"`
	// InputMapping selects declared params from a source Result. Keys are declared
	// param names. Under BodySource previous_result the values are $.-style
	// selectors into the prior Result output (srd028 R12.1, R12.2); under
	// BodySource command_state they are $from(label).path selectors into a labeled
	// prior step's output in the command-state store (srd028 R13.1). Selector form
	// must match the body_source (rest-tool-format V32).
	InputMapping map[string]string `yaml:"input_mapping,omitempty"`
	// CarryForward names declared params copied into this operation's Result
	// output under a carried key so a later word can select them (srd028 R12.3).
	CarryForward []string `yaml:"carry_forward,omitempty"`
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
