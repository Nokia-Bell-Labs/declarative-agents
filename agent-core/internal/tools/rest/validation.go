// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

const (
	authNone        = "none"
	authBasic       = "basic"
	authBearer      = "bearer"
	authHeaderToken = "header_token"
	authQueryToken  = "query_token"

	redirectNone      = "none"
	redirectSameHost  = "same_host"
	redirectAllowlist = "allowlist"

	bindingDynamicSignal = "emit_dynamic_signal"
)

var (
	bodyParamPattern     = regexp.MustCompile(`params\.([A-Za-z_][A-Za-z0-9_]*)`)
	pathParamPattern     = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)(?:\.\.\.)?\}`)
	pathParamFullPattern = regexp.MustCompile(`^\{([A-Za-z_][A-Za-z0-9_]*)(\.\.\.)?\}$`)
)

// ValidateDefinition validates a declarative REST definition before use.
func ValidateDefinition(def Definition) error {
	if def.Version == "" {
		return fmt.Errorf("rest.version is required")
	}
	if err := validateAuthProfiles(def.Auth); err != nil {
		return err
	}
	if err := validateLimitProfiles(def.Limits); err != nil {
		return err
	}
	if err := validateOpenAPINameCollisions(def); err != nil {
		return err
	}
	if err := validateClients(def.Clients, def.RetryPolicies); err != nil {
		return err
	}
	return validateServers(def.Servers, def.Limits)
}

func validateAuthProfiles(profiles map[string]AuthProfile) error {
	for name, profile := range profiles {
		switch profile.Type {
		case authNone, authBasic, authBearer, authHeaderToken, authQueryToken:
			continue
		default:
			return fmt.Errorf("auth profile %q has unsupported type %q", name, profile.Type)
		}
	}
	return nil
}

func validateLimitProfiles(profiles map[string]LimitProfile) error {
	for name, profile := range profiles {
		mode := profile.Redirect.Mode
		if mode == "" {
			continue
		}
		switch mode {
		case redirectNone, redirectSameHost, redirectAllowlist:
			continue
		default:
			return fmt.Errorf("limit profile %q has unsupported redirect mode %q", name, mode)
		}
	}
	return nil
}

func validateOpenAPINameCollisions(def Definition) error {
	operationNames := map[string]string{}
	endpointNames := map[string]string{}
	for clientName, client := range def.Clients {
		for name := range client.Operations {
			if err := addUnique(operationNames, name, "client "+clientName); err != nil {
				return err
			}
		}
	}
	for serverName, server := range def.Servers {
		for name := range server.Endpoints {
			if err := addUnique(endpointNames, name, "server "+serverName); err != nil {
				return err
			}
		}
	}
	return validateImportNames(def.OpenAPI, operationNames, endpointNames)
}

func validateImportNames(imports map[string]OpenAPIImport, operations, endpoints map[string]string) error {
	for importName, imp := range imports {
		for _, operationID := range imp.Expose {
			if err := addUnique(operations, operationID, "openapi "+importName); err != nil {
				return err
			}
		}
		for operationID, endpointName := range imp.Bind {
			if operationID == "" {
				return fmt.Errorf("openapi %q bind contains an empty operation ID", importName)
			}
			if err := addUnique(endpoints, endpointName, "openapi "+importName); err != nil {
				return err
			}
		}
	}
	return nil
}

func addUnique(seen map[string]string, name, owner string) error {
	if name == "" {
		return fmt.Errorf("%s contains an empty REST name", owner)
	}
	if previous, ok := seen[name]; ok {
		return fmt.Errorf("REST name %q is defined by both %s and %s", name, previous, owner)
	}
	seen[name] = owner
	return nil
}

func validateClients(clients map[string]Client, retries map[string]RetryPolicy) error {
	for clientName, client := range clients {
		for resourceName, resource := range client.Resources {
			if err := validateResource(clientName, resourceName, resource, retries[client.RetryRef]); err != nil {
				return err
			}
		}
		for operationName, operation := range client.Operations {
			if err := validateOperation(operationName, operation, false); err != nil {
				return err
			}
			if err := validateAsyncRetry(operationName, operation, retries[client.RetryRef]); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateResource(clientName, resourceName string, resource Resource, retry RetryPolicy) error {
	for verb, operation := range resource.Operations {
		if !isResourceVerb(verb) {
			return fmt.Errorf("resource %s.%s uses unsupported operation %q", clientName, resourceName, verb)
		}
		if operation.Path == "" {
			operation.Path = resource.Path
		}
		if err := validateOperation(resourceName+"."+verb, operation, isMutatingVerb(verb)); err != nil {
			return err
		}
		if err := validateAsyncRetry(resourceName+"."+verb, operation, retry); err != nil {
			return err
		}
	}
	return nil
}

func validateOperation(name string, operation Operation, mutatingResource bool) error {
	if err := validateDeclaredInputs(name, operation); err != nil {
		return err
	}
	if isMutatingOperation(operation, mutatingResource) {
		if err := validateMutatingOperation(name, operation); err != nil {
			return err
		}
	}
	if operation.Async != nil {
		return validateAsyncOperation(name, *operation.Async)
	}
	return validateResponseMapping(name, operation.Response)
}

func validateDeclaredInputs(name string, operation Operation) error {
	params, err := pathParams("operation "+name, operation.Path)
	if err != nil {
		return err
	}
	for _, param := range params {
		if _, ok := operation.Params.Path[param.name]; !ok {
			return fmt.Errorf("operation %q path param %q is not declared", name, param.name)
		}
	}
	for _, field := range bodyTemplateFields(operation.Body) {
		if !operation.Params.declares(field) {
			return fmt.Errorf("operation %q body references undeclared param %q", name, field)
		}
	}
	return nil
}

func validateMutatingOperation(name string, operation Operation) error {
	if len(operation.SideEffects) == 0 {
		return fmt.Errorf("operation %q mutates state without side_effects", name)
	}
	if operation.Reversibility.Classification == "" {
		return fmt.Errorf("operation %q mutates state without reversibility", name)
	}
	if operation.Reversibility.Classification == "irreversible" && !operation.Reversibility.RequiresConfirmation {
		return fmt.Errorf("operation %q is irreversible without confirmation", name)
	}
	return nil
}

func validateAsyncOperation(name string, async AsyncClientConfig) error {
	if async.RequestID == "" {
		return fmt.Errorf("operation %q async config requires request_id", name)
	}
	if async.Timeout == "" {
		return fmt.Errorf("operation %q async config requires timeout", name)
	}
	return nil
}

func validateAsyncRetry(name string, operation Operation, retry RetryPolicy) error {
	if operation.Async == nil || !retryRequiresIdempotency(retry) {
		return nil
	}
	if operation.Async.IdempotencyToken == "" {
		return fmt.Errorf("operation %q async retry requires idempotency metadata", name)
	}
	return nil
}

func retryRequiresIdempotency(retry RetryPolicy) bool {
	if !retry.RequireIdempotency {
		return false
	}
	return retry.Attempts > 1 || len(retry.RetryStatus) > 0 || retry.RetryNetworkErrors
}

func validateServers(servers map[string]Server, limits map[string]LimitProfile) error {
	for serverName, server := range servers {
		if err := validatePublicListener(serverName, server, limits); err != nil {
			return err
		}
		for endpointName, endpoint := range server.Endpoints {
			if err := validateEndpoint(endpointName, endpoint); err != nil {
				return err
			}
		}
	}
	return nil
}

func validatePublicListener(name string, server Server, limits map[string]LimitProfile) error {
	if !isPublicListener(server.Address) {
		return nil
	}
	limit, ok := limits[server.LimitsRef]
	if ok && limit.Network.AllowPublicListener {
		return nil
	}
	return fmt.Errorf("server %q binds public address without allow_public_listener", name)
}

func validateEndpoint(name string, endpoint Endpoint) error {
	if endpoint.Binding == bindingDynamicSignal && len(endpoint.AllowedSignals) == 0 {
		return fmt.Errorf("endpoint %q emit_dynamic_signal requires allowed_signals", name)
	}
	if err := validateMonitorView(name, endpoint); err != nil {
		return err
	}
	if endpoint.Binding == bindingMachineRequest {
		if err := validateMachineRequestEndpoint(name, endpoint); err != nil {
			return err
		}
	}
	params, err := pathParams("endpoint "+name, endpoint.Path)
	if err != nil {
		return err
	}
	for _, param := range params {
		if _, ok := endpoint.Request.Path[param.name]; !ok {
			return fmt.Errorf("endpoint %q path param %q is not declared", name, param.name)
		}
	}
	return validateResponseMapping(name, endpoint.Response)
}

func validateMonitorView(name string, endpoint Endpoint) error {
	if endpoint.MonitorView == "" {
		return nil
	}
	switch endpoint.Binding {
	case bindingReadState, bindingStaticMetadata, bindingStreamEvents:
	default:
		return fmt.Errorf("endpoint %q monitor_view requires read_state, static_metadata, or stream_events binding", name)
	}
	switch endpoint.MonitorView {
	case monitorViewMachine, monitorViewState, monitorViewTools, monitorViewMetrics, monitorViewEvents, "openapi":
		return nil
	default:
		return fmt.Errorf("endpoint %q has unsupported monitor_view %q", name, endpoint.MonitorView)
	}
}

func validateMachineRequestEndpoint(name string, endpoint Endpoint) error {
	cfg := endpoint.MachineRequest
	if cfg.Profile == "" && cfg.Machine == "" && cfg.MachineSpec == nil {
		return fmt.Errorf("endpoint %q machine_request requires profile, machine, or machine spec", name)
	}
	if len(cfg.Response.TerminalSignals) == 0 {
		return fmt.Errorf("endpoint %q machine_request requires response terminal_signals", name)
	}
	if cfg.Timeout == "" {
		return fmt.Errorf("endpoint %q machine_request requires timeout", name)
	}
	return nil
}

type pathParam struct {
	name     string
	catchAll bool
}

func pathParams(owner, path string) ([]pathParam, error) {
	seen := map[string]bool{}
	parts := pathSegments(path)
	params := []pathParam{}
	for i, segment := range parts {
		param, ok, err := pathParamSegment(owner, segment)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if seen[param.name] {
			return nil, fmt.Errorf("%s path param %q is ambiguous", owner, param.name)
		}
		if param.catchAll && i != len(parts)-1 {
			return nil, fmt.Errorf("%s catch-all path param %q must be final", owner, param.name)
		}
		seen[param.name] = true
		params = append(params, param)
	}
	return params, nil
}

func pathParamSegment(owner, segment string) (pathParam, bool, error) {
	if !strings.ContainsAny(segment, "{}") {
		return pathParam{}, false, nil
	}
	match := pathParamFullPattern.FindStringSubmatch(segment)
	if match == nil {
		return pathParam{}, false, fmt.Errorf("%s has malformed path param segment %q", owner, segment)
	}
	return pathParam{name: match[1], catchAll: match[2] == "..."}, true, nil
}

func validateResponseMapping(name string, mapping ResponseMapping) error {
	for _, selector := range mapping.Redact {
		if !validSelector(selector) {
			return fmt.Errorf("%q has invalid redaction selector %q", name, selector)
		}
	}
	return nil
}

func bodyTemplateFields(value interface{}) []string {
	var fields []string
	collectBodyTemplateFields(value, &fields)
	return fields
}

func collectBodyTemplateFields(value interface{}, fields *[]string) {
	switch typed := value.(type) {
	case string:
		for _, match := range bodyParamPattern.FindAllStringSubmatch(typed, -1) {
			*fields = append(*fields, match[1])
		}
	case []interface{}:
		for _, item := range typed {
			collectBodyTemplateFields(item, fields)
		}
	case map[string]interface{}:
		for _, item := range typed {
			collectBodyTemplateFields(item, fields)
		}
	}
}

func (b RequestBinding) declares(name string) bool {
	if _, ok := b.Path[name]; ok {
		return true
	}
	if _, ok := b.Query[name]; ok {
		return true
	}
	if _, ok := b.Headers[name]; ok {
		return true
	}
	return bodySchemaDeclares(b.BodySchema, name)
}

func bodySchemaDeclares(schema map[string]interface{}, name string) bool {
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		return false
	}
	_, ok = props[name]
	return ok
}

func isResourceVerb(verb string) bool {
	switch verb {
	case "get", "set", "create", "delete":
		return true
	default:
		return false
	}
}

func isMutatingVerb(verb string) bool {
	return verb == "set" || verb == "create" || verb == "delete"
}

func isMutatingOperation(operation Operation, resourceMutates bool) bool {
	if resourceMutates {
		return true
	}
	switch strings.ToUpper(operation.Method) {
	case "POST", "PUT", "PATCH", "DELETE":
		return true
	default:
		return false
	}
}

func isPublicListener(address string) bool {
	host := listenerHost(address)
	return host == "" || host == "0.0.0.0" || host == "::" || host == "[::]"
}

func listenerHost(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err == nil {
		return host
	}
	return address
}

func validSelector(selector string) bool {
	switch {
	case strings.HasPrefix(selector, "$."):
		return len(selector) > 2
	case strings.HasPrefix(selector, "headers."):
		return len(selector) > len("headers.")
	case strings.HasPrefix(selector, "query."):
		return len(selector) > len("query.")
	case strings.HasPrefix(selector, "body."):
		return len(selector) > len("body.")
	default:
		return false
	}
}
