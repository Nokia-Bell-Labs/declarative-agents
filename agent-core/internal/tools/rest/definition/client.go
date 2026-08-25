// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package definition

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
	OpenAPIOperationID string `yaml:"openapi_operation_id,omitempty"`
	Method             string `yaml:"method,omitempty"`
	Path               string `yaml:"path,omitempty"`
	BaseURLSource      string `yaml:"base_url_source,omitempty"`
	BaseURLSelector    string `yaml:"base_url_selector,omitempty"`
	// BaseURLHostSelector selects a bare host or IP that is composed with
	// BaseURLScheme and BaseURLPort into the request base URL, for a target
	// discovered per item rather than published as a whole URL (srd028 R14.6).
	// It is mutually exclusive with BaseURLSelector.
	BaseURLHostSelector string `yaml:"base_url_host_selector,omitempty"`
	BaseURLScheme       string `yaml:"base_url_scheme,omitempty"`
	BaseURLPort         string `yaml:"base_url_port,omitempty"`
	// BaseURLPortSelector resolves the composed authority's port from command
	// state, for a fleet whose targets do not share one port. The client's
	// network ports allowlist still authorizes the result (srd028 R14.6).
	BaseURLPortSelector string                 `yaml:"base_url_port_selector,omitempty"`
	AllowSelectedAuth   bool                   `yaml:"allow_auth_on_selected_authority,omitempty"`
	Params              RequestBinding         `yaml:"params,omitempty"`
	Body                map[string]interface{} `yaml:"body,omitempty"`
	Success             StatusMapping          `yaml:"success"`
	Failures            []StatusMapping        `yaml:"failures,omitempty"`
	ResponseRef         string                 `yaml:"response_ref,omitempty"`
	Response            ResponseMapping        `yaml:"response,omitempty"`
	SideEffects         []SideEffect           `yaml:"side_effects,omitempty"`
	Reversibility       Reversibility          `yaml:"reversibility,omitempty"`
	Compensation        map[string]interface{} `yaml:"compensation,omitempty"`
	Async               *AsyncClientConfig     `yaml:"async,omitempty"`
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

// AsyncClientConfig enables send and await behavior for an operation.
type AsyncClientConfig struct {
	RequestID        string `yaml:"request_id"`
	Correlation      string `yaml:"correlation,omitempty"`
	IdempotencyToken string `yaml:"idempotency_token,omitempty"`
	AwaitOperation   string `yaml:"await_operation,omitempty"`
	Timeout          string `yaml:"timeout,omitempty"`
	StateRetention   string `yaml:"state_retention,omitempty"`
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

// Declares reports whether name is a declared path, query, header, or body field.
func (b RequestBinding) Declares(name string) bool {
	if _, ok := b.Path[name]; ok {
		return true
	}
	if _, ok := b.Query[name]; ok {
		return true
	}
	if _, ok := b.Headers[name]; ok {
		return true
	}
	props, _ := b.BodySchema["properties"].(map[string]interface{})
	_, ok := props[name]
	return ok
}
