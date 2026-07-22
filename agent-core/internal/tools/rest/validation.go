// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
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
	bindingStaticAssets  = "static_assets"
	bindingRedirect      = "redirect"

	queueOverflowReject     = "reject"
	queueOverflowDropOldest = "drop_oldest"
	queueOverflowDropNewest = "drop_newest"

	shutdownPolicyDrain         = "drain"
	shutdownPolicyDrainThenStop = "drain_then_stop"
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
	if err := validateReservedDocumentResources(def); err != nil {
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
		if mode != "" && !validRedirectMode(mode) {
			return fmt.Errorf("limit profile %q has unsupported redirect mode %q", name, mode)
		}
		if err := validateCIDRConfig(name, profile.Network.CIDRs); err != nil {
			return err
		}
	}
	return nil
}

func validRedirectMode(mode string) bool {
	return mode == redirectNone || mode == redirectSameHost || mode == redirectAllowlist
}

func validateCIDRConfig(profile string, cidrs []string) error {
	for _, raw := range cidrs {
		if _, _, err := net.ParseCIDR(raw); err != nil {
			return fmt.Errorf("limit profile %q has invalid CIDR %q", profile, raw)
		}
	}
	return nil
}

func validateReservedDocumentResources(def Definition) error {
	if len(def.DocumentResources) > 0 {
		return fmt.Errorf("rest.document_resources is a reserved target-format field; current REST loading rejects it until generic document resource loading is implemented")
	}
	return validateMachineRequestDocumentResources(def.Servers)
}

func validateMachineRequestDocumentResources(servers map[string]Server) error {
	for serverName, server := range servers {
		for endpointName, endpoint := range server.Endpoints {
			if len(endpoint.MachineRequest.DocumentResources) > 0 {
				return fmt.Errorf(
					"server %q endpoint %q machine_request.document_resources is a reserved target-format field; current request machines use profile-selected filesystem resource ToolDefs",
					serverName,
					endpointName,
				)
			}
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
			if err := validateResource(clientName, resourceName, resource, retries[client.RetryRef], client.Operations); err != nil {
				return err
			}
		}
		for operationName, operation := range client.Operations {
			if err := validateOperation(operationName, operation, false, client.Operations); err != nil {
				return err
			}
			if err := validateAsyncRetry(operationName, operation, retries[client.RetryRef]); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateResource(clientName, resourceName string, resource Resource, retry RetryPolicy, clientOps map[string]Operation) error {
	for verb, operation := range resource.Operations {
		if !isResourceVerb(verb) {
			return fmt.Errorf("resource %s.%s uses unsupported operation %q", clientName, resourceName, verb)
		}
		if operation.Path == "" {
			operation.Path = resource.Path
		}
		if err := validateOperation(resourceName+"."+verb, operation, isMutatingVerb(verb), clientOps); err != nil {
			return err
		}
		if err := validateAsyncRetry(resourceName+"."+verb, operation, retry); err != nil {
			return err
		}
	}
	return nil
}

func validateOperation(name string, operation Operation, mutatingResource bool, clientOps map[string]Operation) error {
	if err := validateDeclaredInputs(name, operation); err != nil {
		return err
	}
	if err := validateRequestBinding(name, operation.Params); err != nil {
		return err
	}
	if err := validateStatusMappings(name, operation); err != nil {
		return err
	}
	if isMutatingOperation(operation, mutatingResource) {
		if err := validateMutatingOperation(name, operation); err != nil {
			return err
		}
	}
	if operation.Async != nil {
		return validateAsyncOperation(name, *operation.Async, clientOps)
	}
	return validateResponseMapping(name, operation.Response)
}

func validateStatusMappings(name string, operation Operation) error {
	owners := make(map[int]string)
	mappings := append([]StatusMapping{operation.Success}, operation.Failures...)
	for index, mapping := range mappings {
		owner := fmt.Sprintf("failure[%d] signal %q", index-1, mapping.Signal)
		if index == 0 {
			owner = fmt.Sprintf("success signal %q", mapping.Signal)
		}
		for _, status := range mapping.Status {
			if previous, exists := owners[status]; exists {
				return fmt.Errorf(
					"operation %q maps HTTP status %d more than once (%s and %s)",
					name, status, previous, owner,
				)
			}
			owners[status] = owner
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

func validateAsyncOperation(name string, async AsyncClientConfig, clientOps map[string]Operation) error {
	if async.RequestID == "" {
		return fmt.Errorf("operation %q async config requires request_id", name)
	}
	if async.Timeout == "" {
		return fmt.Errorf("operation %q async config requires timeout", name)
	}
	if async.AwaitOperation != "" {
		if _, ok := clientOps[async.AwaitOperation]; !ok {
			return fmt.Errorf("operation %q async await_operation %q is not a defined client operation", name, async.AwaitOperation)
		}
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
		if err := validateQueueConfig("server "+serverName, server.Queue); err != nil {
			return err
		}
		if err := validateShutdownConfig(serverName, server.Shutdown); err != nil {
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
	if !handledServerBindings[endpoint.Binding] {
		if endpoint.Binding == "" {
			return fmt.Errorf("endpoint %q has no binding; declare one of: %s", name, sortedServerBindings())
		}
		return fmt.Errorf("endpoint %q has unknown binding %q; the runtime returns 501 for it. Declare one of: %s",
			name, endpoint.Binding, sortedServerBindings())
	}
	if endpoint.Binding == bindingDynamicSignal && len(endpoint.AllowedSignals) == 0 {
		return fmt.Errorf("endpoint %q emit_dynamic_signal requires allowed_signals", name)
	}
	if err := validateQueueConfig("endpoint "+name, endpoint.Queue); err != nil {
		return err
	}
	if endpoint.Binding == bindingLifecycleControl {
		if err := validateLifecycleControlEndpoint(name, endpoint); err != nil {
			return err
		}
	}
	if err := validateMonitorView(name, endpoint); err != nil {
		return err
	}
	if endpoint.Binding == bindingMachineRequest {
		if err := validateMachineRequestEndpoint(name, endpoint); err != nil {
			return err
		}
	}
	if endpoint.StaticAssets != nil && endpoint.Binding != bindingStaticAssets {
		return fmt.Errorf(
			"endpoint %q has static_assets config but binding is %q (want %q)",
			name, endpoint.Binding, bindingStaticAssets,
		)
	}
	if endpoint.Redirect != nil && endpoint.Binding != bindingRedirect {
		return fmt.Errorf(
			"endpoint %q has redirect config but binding is %q (want %q)",
			name, endpoint.Binding, bindingRedirect,
		)
	}
	if endpoint.MonitorProxy != nil && endpoint.Binding != bindingMonitorProxy {
		return fmt.Errorf(
			"endpoint %q has monitor_proxy config but binding is %q (want %q)",
			name, endpoint.Binding, bindingMonitorProxy,
		)
	}
	if endpoint.Binding == bindingMonitorProxy {
		if endpoint.MonitorProxy == nil || len(endpoint.MonitorProxy.Upstreams) == 0 {
			return fmt.Errorf("endpoint %q monitor_proxy requires a non-empty upstreams map", name)
		}
	}
	if endpoint.Binding == bindingStaticAssets {
		if err := validateStaticAssetsEndpoint(name, endpoint); err != nil {
			return err
		}
	}
	if endpoint.Binding == bindingRedirect {
		if err := validateRedirectEndpoint(name, endpoint); err != nil {
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

func validateQueueConfig(owner string, queue QueueConfig) error {
	switch queue.Overflow {
	case "", queueOverflowReject, queueOverflowDropOldest, queueOverflowDropNewest:
		return nil
	default:
		return fmt.Errorf("%s has unsupported queue overflow %q", owner, queue.Overflow)
	}
}

func validateShutdownConfig(name string, shutdown ShutdownConfig) error {
	switch shutdown.DrainPolicy {
	case "", shutdownPolicyDrain, shutdownPolicyDrainThenStop:
	default:
		return fmt.Errorf("server %q has unsupported drain_policy %q", name, shutdown.DrainPolicy)
	}
	if shutdown.DrainTimeout != "" {
		return fmt.Errorf("server %q shutdown.drain_timeout is not supported", name)
	}
	if shutdown.StopListeners != nil && !*shutdown.StopListeners {
		return fmt.Errorf("server %q shutdown.stop_listeners=false is not supported", name)
	}
	if shutdown.QueueOnShutdown != "" {
		return fmt.Errorf("server %q shutdown.queue_on_shutdown is not supported", name)
	}
	if shutdown.UnblockAwaitSignal != "" && shutdown.UnblockAwaitSignal != "ServerStopped" {
		return fmt.Errorf("server %q shutdown.unblock_await_signal is not supported", name)
	}
	return nil
}

func shutdownDrainPolicy(shutdown ShutdownConfig) string {
	if shutdown.DrainPolicy != "" {
		return shutdown.DrainPolicy
	}
	return shutdownPolicyDrainThenStop
}

func validateLifecycleControlEndpoint(name string, endpoint Endpoint) error {
	control := endpoint.LifecycleControl
	switch control.Action {
	case "exit", "pause", "rollback_request", "resume", "inject_signal":
	default:
		return fmt.Errorf("endpoint %q lifecycle_control has unsupported action %q", name, control.Action)
	}
	if control.Action == "inject_signal" && len(control.AllowedSignals) == 0 {
		return fmt.Errorf("endpoint %q lifecycle_control inject_signal requires allowed_signals", name)
	}
	if control.Action != "inject_signal" && lifecycleSignal(endpoint) == "" {
		return fmt.Errorf("endpoint %q lifecycle_control requires signal", name)
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

func validateStaticAssetsEndpoint(name string, endpoint Endpoint) error {
	if endpoint.StaticAssets == nil {
		return fmt.Errorf("endpoint %q static_assets binding requires static_assets config", name)
	}
	if strings.TrimSpace(endpoint.StaticAssets.Root) == "" {
		return fmt.Errorf("endpoint %q static_assets requires non-empty root", name)
	}
	if strings.TrimSpace(endpoint.Method) == "" || strings.ToUpper(strings.TrimSpace(endpoint.Method)) != "GET" {
		return fmt.Errorf("endpoint %q static_assets requires GET method", name)
	}
	return validateStaticAssetsNoConflicts(name, endpoint)
}

func validateStaticAssetsNoConflicts(name string, endpoint Endpoint) error {
	return validateInboundAssetLikeNoConflicts(name, "static_assets", endpoint)
}

func validateRedirectEndpoint(name string, endpoint Endpoint) error {
	if endpoint.Redirect == nil {
		return fmt.Errorf("endpoint %q redirect binding requires redirect config", name)
	}
	if strings.TrimSpace(endpoint.Redirect.Location) == "" {
		return fmt.Errorf("endpoint %q redirect requires non-empty location", name)
	}
	if strings.TrimSpace(endpoint.Method) == "" || strings.ToUpper(strings.TrimSpace(endpoint.Method)) != "GET" {
		return fmt.Errorf("endpoint %q redirect requires GET method", name)
	}
	if st := endpoint.Redirect.Status; st != 0 && !validInboundRedirectStatus(st) {
		return fmt.Errorf("endpoint %q redirect status must be 301, 302, 303, 307, or 308 (got %d)", name, st)
	}
	return validateInboundAssetLikeNoConflicts(name, "redirect", endpoint)
}

func validInboundRedirectStatus(status int) bool {
	switch status {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return true
	default:
		return false
	}
}

func validateInboundAssetLikeNoConflicts(name, bindingLabel string, endpoint Endpoint) error {
	if endpoint.Signal != "" {
		return fmt.Errorf("endpoint %q %s must not set signal", name, bindingLabel)
	}
	if len(endpoint.AllowedSignals) > 0 {
		return fmt.Errorf("endpoint %q %s must not set allowed_signals", name, bindingLabel)
	}
	if lifecycleControlSet(endpoint.LifecycleControl) {
		return fmt.Errorf("endpoint %q %s must not set lifecycle_control", name, bindingLabel)
	}
	if machineRequestYAMLSet(endpoint.MachineRequest) {
		return fmt.Errorf("endpoint %q %s must not set machine_request", name, bindingLabel)
	}
	if endpoint.MonitorView != "" {
		return fmt.Errorf("endpoint %q %s must not set monitor_view", name, bindingLabel)
	}
	if queueConfigSet(endpoint.Queue) {
		return fmt.Errorf("endpoint %q %s must not set queue", name, bindingLabel)
	}
	if endpoint.StaticAssets != nil && bindingLabel == "redirect" {
		return fmt.Errorf("endpoint %q redirect must not set static_assets", name)
	}
	if endpoint.Redirect != nil && bindingLabel == "static_assets" {
		return fmt.Errorf("endpoint %q static_assets must not set redirect", name)
	}
	return nil
}

func lifecycleControlSet(c LifecycleControl) bool {
	return c.Action != "" || c.Signal != "" || len(c.AllowedSignals) > 0 ||
		len(c.TargetSchema) > 0 || c.RequireAuthRef != ""
}

func machineRequestYAMLSet(cfg MachineRequest) bool {
	if cfg.Profile != "" || cfg.Machine != "" || cfg.InitialSignal != "" || cfg.Timeout != "" {
		return true
	}
	if len(cfg.DocumentResources) > 0 || len(cfg.Response.TerminalSignals) > 0 {
		return true
	}
	m := cfg.Request
	return len(m.Body) > 0 || len(m.Query) > 0 || len(m.Path) > 0 || len(m.Headers) > 0
}

func queueConfigSet(q QueueConfig) bool {
	return q.Name != "" || q.Capacity != 0 || q.Overflow != "" || q.Timeout != "" || len(q.PayloadShape) > 0
}

// validateRequestBinding enforces the previous-Result threading contract:
// a supported body_source, input_mapping only under previous_result, and
// input_mapping and carry_forward that target only declared params and never
// transport authority (srd028 R12.4; rest-tool-format V28-V30).
func validateRequestBinding(name string, binding RequestBinding) error {
	if err := validateBodySource(name, binding.BodySource); err != nil {
		return err
	}
	if len(binding.InputMapping) > 0 && binding.BodySource != bodySourcePreviousResult && binding.BodySource != bodySourceCommandState {
		return fmt.Errorf("operation %q input_mapping requires body_source %s or %s", name, bodySourcePreviousResult, bodySourceCommandState)
	}
	declared := declaredParamNames(binding)
	for target, selector := range binding.InputMapping {
		if forbiddenRuntimeAuthorityFields[target] {
			return fmt.Errorf("operation %q input_mapping target %q cannot set REST authority", name, target)
		}
		if !declared[target] {
			return fmt.Errorf("operation %q input_mapping target %q is not declared", name, target)
		}
		if err := validateSelectorForm(name, binding.BodySource, selector); err != nil {
			return err
		}
	}
	for _, carried := range binding.CarryForward {
		if forbiddenRuntimeAuthorityFields[carried] {
			return fmt.Errorf("operation %q carry_forward entry %q cannot set REST authority", name, carried)
		}
		if !declared[carried] {
			return fmt.Errorf("operation %q carry_forward entry %q is not declared", name, carried)
		}
	}
	return nil
}

func validateBodySource(name, source string) error {
	switch source {
	case "", bodySourceParams, bodySourcePreviousResult, bodySourceNone, bodySourceCommandState:
		// command_state is structurally valid; it is rejected only at runtime when
		// no command-state store view is configured (srd028 R13.5).
		return nil
	default:
		return fmt.Errorf("operation %q has unsupported body_source %q", name, source)
	}
}

// validateSelectorForm enforces rest-tool-format V32: a $from(label).path
// selector is valid only under body_source command_state, and a $.-style selector
// is valid only under body_source previous_result.
func validateSelectorForm(name, source, selector string) error {
	if source == bodySourceCommandState {
		if _, _, ok := core.ParseFromSelector(selector); !ok {
			return fmt.Errorf("operation %q input_mapping selector %q must be a $from(label).path selector under body_source command_state", name, selector)
		}
		return nil
	}
	parsed, ok := core.ParseSelector(selector)
	if !ok || parsed.Label != "" {
		return fmt.Errorf("operation %q input_mapping selector %q must use the $. prefix", name, selector)
	}
	return nil
}

func validateResponseMapping(name string, mapping ResponseMapping) error {
	for _, selector := range mapping.Redact {
		if !validRedactionSelector(selector) {
			return fmt.Errorf("%q has invalid redaction selector %q", name, selector)
		}
	}
	return nil
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
