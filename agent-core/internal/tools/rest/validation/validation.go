// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package validation

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

const (
	authNone        = "none"
	authBasic       = "basic"
	authBearer      = "bearer"
	authHeaderToken = "header_token"
	authQueryToken  = "query_token"

	bodySourceParams         = "params"
	bodySourcePreviousResult = "previous_result"
	bodySourceNone           = "none"
	bodySourceCommandState   = "command_state"

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
	if err := validateRetryPolicies(def.RetryPolicies); err != nil {
		return err
	}
	if err := validateClients(def.Clients, def.RetryPolicies, def.ResponseMappings); err != nil {
		return err
	}
	if err := validateServers(def.Servers, def.Limits); err != nil {
		return err
	}
	return validateServerAuthRefs(def.Servers, def.Auth)
}

func validateServerAuthRefs(servers map[string]Server, auth map[string]AuthProfile) error {
	for name, server := range servers {
		if ref := server.LifecycleExit.AuthRef; ref != "" {
			if _, ok := auth[ref]; !ok {
				return fmt.Errorf("server %q lifecycle_exit references unknown auth profile %q", name, ref)
			}
		}
		for endpointName, endpoint := range server.Endpoints {
			ref := endpoint.LifecycleControl.RequireAuthRef
			if ref != "" {
				if _, ok := auth[ref]; !ok {
					return fmt.Errorf(
						"server %q endpoint %q references unknown lifecycle auth profile %q",
						name, endpointName, ref,
					)
				}
			}
		}
	}
	return nil
}

// validateRetryPolicies rejects an unsupported backoff or an unparseable delay
// at load, so a typo fails there rather than silently degrading to a flat or
// zero delay at dispatch (GH-1379). The runtime honors backoff and max_delay
// (client_command.retryDelay), so both are validated here.
func validateRetryPolicies(policies map[string]RetryPolicy) error {
	for name, retry := range policies {
		if retry.Attempts <= 0 {
			return fmt.Errorf("retry policy %q attempts must be positive", name)
		}
		switch retry.Backoff {
		case "", "none", "fixed", "exponential":
		default:
			return fmt.Errorf("retry policy %q has unsupported backoff %q; want none, fixed, or exponential", name, retry.Backoff)
		}
		if err := validateOptionalDuration(retry.InitialDelay); err != nil {
			return fmt.Errorf("retry policy %q initial_delay %q: %w", name, retry.InitialDelay, err)
		}
		if err := validateOptionalDuration(retry.MaxDelay); err != nil {
			return fmt.Errorf("retry policy %q max_delay %q: %w", name, retry.MaxDelay, err)
		}
	}
	return nil
}

// RetryAggregateTimeout returns the conservative dispatch authority for one
// retrying HTTP operation: every bounded attempt plus the delay before each
// retry. It calls retryDelay so static inspection and runtime execution share
// fixed, exponential, max-delay, and overflow-saturation semantics.
func RetryAggregateTimeout(attemptTimeout time.Duration, retry RetryPolicy) (time.Duration, error) {
	if attemptTimeout <= 0 {
		return 0, fmt.Errorf("attempt timeout must be positive")
	}
	if err := validateRetryPolicies(map[string]RetryPolicy{"selected": retry}); err != nil {
		return 0, err
	}
	total, err := checkedDurationProduct(attemptTimeout, retry.Attempts)
	if err != nil {
		return 0, fmt.Errorf("retry attempt timeout aggregate: %w", err)
	}
	retries := retry.Attempts - 1
	if retries == 0 || retry.Backoff == "none" {
		return total, nil
	}
	return addRetryDelayAggregate(total, retries, retry)
}

func addRetryDelayAggregate(
	total time.Duration,
	retries int,
	retry RetryPolicy,
) (time.Duration, error) {
	if retry.Backoff == "" || retry.Backoff == "fixed" {
		delays, err := checkedDurationProduct(retryDelay(retry, 1), retries)
		if err != nil {
			return 0, fmt.Errorf("retry delay aggregate: %w", err)
		}
		return checkedDurationSum(total, delays)
	}

	maxDelay := parseDuration(retry.MaxDelay, 0)
	for attempt := 1; attempt <= retries; attempt++ {
		delay := retryDelay(retry, attempt)
		next, err := checkedDurationSum(total, delay)
		if err != nil {
			return 0, fmt.Errorf("retry delay aggregate: %w", err)
		}
		total = next
		remaining := retries - attempt
		if remaining == 0 || delay == 0 {
			return total, nil
		}
		if maxDelay > 0 && delay == maxDelay {
			delays, productErr := checkedDurationProduct(delay, remaining)
			if productErr != nil {
				return 0, fmt.Errorf("retry delay aggregate: %w", productErr)
			}
			return checkedDurationSum(total, delays)
		}
	}
	return total, nil
}

func checkedDurationProduct(value time.Duration, count int) (time.Duration, error) {
	if value < 0 || count < 0 {
		return 0, fmt.Errorf("duration and count must be non-negative")
	}
	if value == 0 || count == 0 {
		return 0, nil
	}
	if value > time.Duration(1<<63-1)/time.Duration(count) {
		return 0, fmt.Errorf("duration overflow")
	}
	return value * time.Duration(count), nil
}

func checkedDurationSum(left, right time.Duration) (time.Duration, error) {
	if left < 0 || right < 0 {
		return 0, fmt.Errorf("durations must be non-negative")
	}
	if left > time.Duration(1<<63-1)-right {
		return 0, fmt.Errorf("duration overflow")
	}
	return left + right, nil
}

func validateOptionalDuration(value string) error {
	if value == "" {
		return nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return err
	}
	if duration < 0 {
		return fmt.Errorf("must be non-negative")
	}
	return nil
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

func validateClients(
	clients map[string]Client,
	retries map[string]RetryPolicy,
	responseMappings map[string]ResponseMapping,
) error {
	for clientName, client := range clients {
		if client.RetryRef != "" {
			if _, ok := retries[client.RetryRef]; !ok {
				return fmt.Errorf("REST client %q references undefined retry policy %q", clientName, client.RetryRef)
			}
		}
		for resourceName, resource := range client.Resources {
			if err := validateResource(
				clientName, resourceName, resource, retries[client.RetryRef], responseMappings,
			); err != nil {
				return err
			}
		}
		for operationName, operation := range client.Operations {
			if err := validateOperation(
				operationName, operation, false, client.Operations, responseMappings,
			); err != nil {
				return err
			}
			if err := validateAsyncRetry(operationName, operation, retries[client.RetryRef]); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateResource(
	clientName string,
	resourceName string,
	resource Resource,
	retry RetryPolicy,
	responseMappings map[string]ResponseMapping,
) error {
	if resource.IDField != "" || resource.VersionField != "" {
		return fmt.Errorf(
			"resource %s.%s id_field and version_field are reserved and not implemented",
			clientName, resourceName,
		)
	}
	for verb, operation := range resource.Operations {
		if !isResourceVerb(verb) {
			return fmt.Errorf("resource %s.%s uses unsupported operation %q", clientName, resourceName, verb)
		}
		if operation.Path == "" {
			operation.Path = resource.Path
		}
		if err := validateOperation(
			resourceName+"."+verb, operation, isMutatingVerb(verb), resource.Operations, responseMappings,
		); err != nil {
			return err
		}
		if err := validateAsyncRetry(resourceName+"."+verb, operation, retry); err != nil {
			return err
		}
	}
	return nil
}

func validateOperation(
	name string,
	operation Operation,
	mutatingResource bool,
	clientOps map[string]Operation,
	responseMappings map[string]ResponseMapping,
) error {
	if err := validateDeclaredInputs(name, operation); err != nil {
		return err
	}
	if err := validateBaseURLSource(name, operation); err != nil {
		return err
	}
	if err := validateRequestBinding(name, operation.Params); err != nil {
		return err
	}
	if err := validateStatusMappings(name, operation); err != nil {
		return err
	}
	if isMutatingOperation(operation, mutatingResource) {
		if err := validateMutatingOperation(name, operation, clientOps, responseMappings); err != nil {
			return err
		}
	}
	if operation.Async != nil {
		return validateAsyncOperation(name, *operation.Async, clientOps)
	}
	return validateResponseMapping(name, operation.Response)
}

func validateBaseURLSource(name string, operation Operation) error {
	switch operation.BaseURLSource {
	case "":
		return validateStaticBaseURLTarget(name, operation)
	case bodySourceCommandState:
		return validateSelectedBaseURLTarget(name, operation)
	default:
		return fmt.Errorf("operation %q has unsupported base_url_source %q", name, operation.BaseURLSource)
	}
}

// validateStaticBaseURLTarget rejects target-selection fields on an operation
// that keeps its client's configured base_url (srd028 R14.1).
func validateStaticBaseURLTarget(name string, operation Operation) error {
	if operation.BaseURLSelector != "" {
		return fmt.Errorf("operation %q base_url_selector requires base_url_source command_state", name)
	}
	if operation.BaseURLHostSelector != "" {
		return fmt.Errorf("operation %q base_url_host_selector requires base_url_source command_state", name)
	}
	if err := validateComposedTargetFields(name, operation); err != nil {
		return err
	}
	if operation.AllowSelectedAuth {
		return fmt.Errorf("operation %q allow_auth_on_selected_authority requires base_url_source command_state", name)
	}
	return nil
}

// validateSelectedBaseURLTarget accepts exactly one selector form: a whole URL
// or a bare host composed with a declared scheme and port (srd028 R14.1, R14.6).
func validateSelectedBaseURLTarget(name string, operation Operation) error {
	if operation.BaseURLSelector != "" && operation.BaseURLHostSelector != "" {
		return fmt.Errorf(
			"operation %q declares both base_url_selector and base_url_host_selector; declare one", name)
	}
	if operation.BaseURLHostSelector != "" {
		return validateHostSelectorTarget(name, operation)
	}
	if _, _, ok := core.ParseFromSelector(operation.BaseURLSelector); !ok {
		return fmt.Errorf("operation %q base_url_selector %q must be a $from(label).path selector under base_url_source command_state", name, operation.BaseURLSelector)
	}
	return validateComposedTargetFields(name, operation)
}

func validateHostSelectorTarget(name string, operation Operation) error {
	if _, _, ok := core.ParseFromSelector(operation.BaseURLHostSelector); !ok {
		return fmt.Errorf("operation %q base_url_host_selector %q must be a $from(label).path selector under base_url_source command_state", name, operation.BaseURLHostSelector)
	}
	switch operation.BaseURLScheme {
	case "", "http", "https":
	default:
		return fmt.Errorf("operation %q has unsupported base_url_scheme %q", name, operation.BaseURLScheme)
	}
	return validateComposedPort(name, operation)
}

// validateComposedPort accepts one port form: a declared literal or a selector
// resolved per item (srd028 R14.6).
func validateComposedPort(name string, operation Operation) error {
	if operation.BaseURLPort != "" && operation.BaseURLPortSelector != "" {
		return fmt.Errorf(
			"operation %q declares both base_url_port and base_url_port_selector; declare one", name)
	}
	if operation.BaseURLPortSelector != "" {
		if _, _, ok := core.ParseFromSelector(operation.BaseURLPortSelector); !ok {
			return fmt.Errorf("operation %q base_url_port_selector %q must be a $from(label).path selector under base_url_source command_state", name, operation.BaseURLPortSelector)
		}
		return nil
	}
	return validateBaseURLPort(name, operation.BaseURLPort)
}

// validateComposedTargetFields rejects a declared scheme or port without the
// host selector they compose with.
func validateComposedTargetFields(name string, operation Operation) error {
	if operation.BaseURLScheme != "" || operation.BaseURLPort != "" || operation.BaseURLPortSelector != "" {
		return fmt.Errorf(
			"operation %q base_url_scheme, base_url_port, and base_url_port_selector require base_url_host_selector", name)
	}
	return nil
}

func validateBaseURLPort(name, port string) error {
	if port == "" {
		return nil
	}
	if !portInRange(port) {
		return fmt.Errorf("operation %q has invalid base_url_port %q", name, port)
	}
	return nil
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

func validateMutatingOperation(
	name string,
	operation Operation,
	operations map[string]Operation,
	responseMappings map[string]ResponseMapping,
) error {
	if len(operation.SideEffects) == 0 {
		return fmt.Errorf("operation %q mutates state without side_effects", name)
	}
	if operation.Reversibility.Classification == "" {
		return fmt.Errorf("operation %q mutates state without reversibility", name)
	}
	if operation.Reversibility.Classification == "irreversible" && !operation.Reversibility.RequiresConfirmation {
		return fmt.Errorf("operation %q is irreversible without confirmation", name)
	}
	if operation.Reversibility.Classification == "compensatable" {
		return validateCompensationOperation(name, operation, operations, responseMappings)
	}
	return nil
}

func validateCompensationOperation(
	name string,
	operation Operation,
	operations map[string]Operation,
	responseMappings map[string]ResponseMapping,
) error {
	if len(operation.Compensation) == 0 {
		return fmt.Errorf("operation %q is compensatable without an executable compensation target", name)
	}
	targetName, ok := operation.Compensation["operation"].(string)
	if !ok || strings.TrimSpace(targetName) == "" {
		return fmt.Errorf("operation %q compensation requires a non-empty operation target", name)
	}
	target, ok := operations[targetName]
	if !ok {
		return fmt.Errorf("operation %q compensation target %q is not defined in the same REST operation scope", name, targetName)
	}
	if target.BaseURLSource == bodySourceCommandState || target.Params.BodySource == bodySourceCommandState {
		return fmt.Errorf("operation %q compensation target %q requires command_state and cannot execute from a fresh rollback receipt", name, targetName)
	}
	configured, err := compensationParameterMapping(name, operation.Compensation, target.Params)
	if err != nil {
		return err
	}
	produced := compensationProducedParams(operation, target, configured, responseMappings)
	for required := range requiredOperationParams(target) {
		if !produced[required] {
			return fmt.Errorf(
				"operation %q compensation target %q requires parameter %q that the rollback receipt cannot produce",
				name, targetName, required,
			)
		}
	}
	return nil
}

func compensationParameterMapping(
	name string,
	compensation map[string]interface{},
	target RequestBinding,
) (map[string]interface{}, error) {
	raw, exists := compensation["parameters"]
	if !exists {
		return nil, nil
	}
	parameters, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("operation %q compensation parameters must be a mapping", name)
	}
	declared := declaredParamNames(target)
	for parameter := range parameters {
		if !declared[parameter] {
			return nil, fmt.Errorf(
				"operation %q compensation parameter %q is not declared by its target",
				name, parameter,
			)
		}
	}
	return parameters, nil
}

func compensationProducedParams(
	source Operation,
	target Operation,
	configured map[string]interface{},
	responseMappings map[string]ResponseMapping,
) map[string]bool {
	produced := requiredOperationParams(source)
	for name, value := range configured {
		if compensationMappingValueAvailable(value) {
			produced[name] = true
		}
	}
	if operationProducesResourceID(source, produced, responseMappings) {
		for _, alias := range []string{"resource_id", "id", "number"} {
			produced[alias] = true
		}
	}
	if operationProducesRequestID(source, produced, responseMappings) {
		produced["request_id"] = true
	}
	if source.Async != nil && source.Async.IdempotencyToken != "" {
		produced["idempotency_token"] = true
	}

	switch target.Params.BodySource {
	case bodySourceNone:
		return map[string]bool{}
	case bodySourcePreviousResult:
		selected := map[string]bool{}
		for name, selector := range target.Params.InputMapping {
			parsed, ok := core.ParseSelector(selector)
			if ok && parsed.Label == "" && len(parsed.Path) == 1 && produced[parsed.Path[0]] {
				selected[name] = true
			}
		}
		return selected
	default:
		return produced
	}
}

func compensationMappingValueAvailable(value interface{}) bool {
	if value == nil {
		return false
	}
	text, ok := value.(string)
	return !ok || strings.TrimSpace(text) != ""
}

func requiredOperationParams(operation Operation) map[string]bool {
	required := map[string]bool{}
	for name := range operation.Params.Path {
		required[name] = true
	}
	for _, name := range bodyRequiredFields(operation.Params.BodySchema) {
		required[name] = true
	}
	return required
}

func bodyRequiredFields(schema map[string]interface{}) []string {
	var required []string
	switch values := schema["required"].(type) {
	case []interface{}:
		for _, value := range values {
			if name, ok := value.(string); ok && name != "" {
				required = append(required, name)
			}
		}
	case []string:
		required = append(required, values...)
	}
	return required
}

func operationProducesResourceID(
	operation Operation,
	produced map[string]bool,
	responseMappings map[string]ResponseMapping,
) bool {
	if resolvedResponseMapping(
		ClientOperationDefinition{Operation: operation, ResponseMappings: responseMappings},
		operation.Success,
	).ResourceID != "" {
		return true
	}
	return produced["resource_id"] || produced["id"] || produced["number"]
}

func operationProducesRequestID(
	operation Operation,
	produced map[string]bool,
	responseMappings map[string]ResponseMapping,
) bool {
	if resolvedResponseMapping(
		ClientOperationDefinition{Operation: operation, ResponseMappings: responseMappings},
		operation.Success,
	).RequestID != "" || produced["request_id"] {
		return true
	}
	return operation.Async != nil && operation.Async.RequestID != ""
}

func validateAsyncOperation(name string, async AsyncClientConfig, _ map[string]Operation) error {
	if async.RequestID == "" {
		return fmt.Errorf("operation %q async config requires request_id", name)
	}
	if async.Timeout == "" {
		return fmt.Errorf("operation %q async config requires timeout", name)
	}
	if async.AwaitOperation != "" {
		return fmt.Errorf(
			"operation %q async await_operation is unsupported; declare probe and delay states in MachineSpec",
			name,
		)
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
	if endpoint.Binding == bindingDynamicSignal {
		if len(endpoint.SignalMapping) > 0 && endpoint.SignalField == "" {
			return fmt.Errorf("endpoint %q signal_mapping requires signal_field", name)
		}
		for value, signal := range endpoint.SignalMapping {
			if !allowedSignal(signal, endpoint.AllowedSignals) {
				return fmt.Errorf("endpoint %q signal_mapping value %q maps to disallowed signal %q", name, value, signal)
			}
		}
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
	if endpoint.Binding == bindingSignalSource {
		if err := validateSignalSourceEndpoint(name, endpoint); err != nil {
			return err
		}
	} else if signalSourceYAMLSet(endpoint.SignalSource) {
		return fmt.Errorf("endpoint %q has signal_source config but binding is %q", name, endpoint.Binding)
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
	if endpoint.Mock != nil && endpoint.Binding != bindingMock {
		return fmt.Errorf(
			"endpoint %q has mock config but binding is %q (want %q)",
			name, endpoint.Binding, bindingMock,
		)
	}
	// Fixtures are read and checked here so --validate-config rejects a malformed
	// fixture before the server is ever started (srd039 R4.2).
	if endpoint.Binding == bindingMock {
		if _, err := mockRoutes(name, endpoint.Mock); err != nil {
			return err
		}
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
	case "", shutdownPolicyDrainThenStop:
	case shutdownPolicyDrain:
		return fmt.Errorf(
			"server %q shutdown.drain_policy %q is not implemented; use %q",
			name, shutdown.DrainPolicy, shutdownPolicyDrainThenStop,
		)
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
	return nil
}

func validateLifecycleControlEndpoint(name string, endpoint Endpoint) error {
	control := endpoint.LifecycleControl
	switch control.Action {
	case "enqueue_signal", "inject_signal":
	default:
		return fmt.Errorf("endpoint %q lifecycle_control has unsupported action %q", name, control.Action)
	}
	if control.Action == "inject_signal" && len(control.AllowedSignals) == 0 {
		return fmt.Errorf("endpoint %q lifecycle_control inject_signal requires allowed_signals", name)
	}
	if control.Action == "enqueue_signal" && lifecycleSignal(endpoint) == "" {
		return fmt.Errorf("endpoint %q lifecycle_control enqueue_signal requires signal", name)
	}
	return validateResponseMapping(name, endpoint.Response)
}

func validateMonitorView(name string, endpoint Endpoint) error {
	if endpoint.MonitorView == "" {
		if len(endpoint.Labels) > 0 {
			return fmt.Errorf("endpoint %q labels is only valid with monitor_view command_state", name)
		}
		return nil
	}
	switch endpoint.Binding {
	case bindingReadState, bindingStaticMetadata, bindingStreamEvents:
	default:
		return fmt.Errorf("endpoint %q monitor_view requires read_state, static_metadata, or stream_events binding", name)
	}
	switch endpoint.MonitorView {
	case monitorViewCommandState:
		if endpoint.Binding != bindingReadState {
			return fmt.Errorf("endpoint %q monitor_view command_state requires read_state binding", name)
		}
		if len(endpoint.Labels) == 0 {
			return fmt.Errorf("endpoint %q monitor_view command_state requires a non-empty labels allowlist", name)
		}
		return nil
	case monitorViewDeclaredMachines:
		if endpoint.Binding != bindingReadState {
			return fmt.Errorf("endpoint %q monitor_view declared_machines requires read_state binding", name)
		}
		if len(endpoint.Labels) > 0 {
			return fmt.Errorf("endpoint %q labels is only valid with monitor_view command_state", name)
		}
		return nil
	case monitorViewMachine, monitorViewState, monitorViewTools, monitorViewMetrics, monitorViewEvents, "openapi":
		if len(endpoint.Labels) > 0 {
			return fmt.Errorf("endpoint %q labels is only valid with monitor_view command_state", name)
		}
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
	if len(cfg.Response.TerminalSignals) == 0 && len(cfg.Response.TerminalStates) == 0 {
		return fmt.Errorf("endpoint %q machine_request requires response terminal_states or terminal_signals", name)
	}
	if cfg.Timeout == "" {
		return fmt.Errorf("endpoint %q machine_request requires timeout", name)
	}
	if err := validateMachineRequestSensitiveFields(cfg.Request); err != nil {
		return fmt.Errorf("endpoint %q machine_request request: %w", name, err)
	}
	return nil
}

func validateMachineRequestSensitiveFields(mapping MachineRequestMapping) error {
	declared := map[string]bool{}
	for _, fields := range []map[string]string{
		mapping.Body, mapping.Query, mapping.Path, mapping.Headers,
	} {
		for name := range fields {
			declared[name] = true
		}
	}
	seen := map[string]bool{}
	for _, name := range mapping.Sensitive {
		if name == "" {
			return fmt.Errorf("sensitive field name must not be empty")
		}
		if seen[name] {
			return fmt.Errorf("sensitive field %q is duplicated", name)
		}
		if !declared[name] {
			return fmt.Errorf("sensitive field %q is not a mapped request field", name)
		}
		seen[name] = true
	}
	return nil
}

func validateSignalSourceEndpoint(name string, endpoint Endpoint) error {
	cfg := endpoint.SignalSource
	if err := validateSignalSourceNoConflicts(name, endpoint); err != nil {
		return err
	}
	if err := validateSignalSourceIdentity(name, cfg); err != nil {
		return err
	}
	if err := validateSignalSourceSelectors(name, endpoint.Request, cfg); err != nil {
		return err
	}
	if err := validateSignalSourceMappings(name, endpoint.Request, cfg); err != nil {
		return err
	}
	if err := validateSignalSourceSensitive(name, cfg); err != nil {
		return err
	}
	timeout, err := time.ParseDuration(cfg.Timeout)
	if err != nil || timeout <= 0 {
		return fmt.Errorf("endpoint %q signal_source timeout must be a positive duration", name)
	}
	return validateSignalSourceResponses(name, cfg.Responses)
}

func validateSignalSourceNoConflicts(name string, endpoint Endpoint) error {
	if endpoint.Signal != "" || len(endpoint.AllowedSignals) > 0 ||
		endpoint.SignalField != "" || len(endpoint.SignalMapping) > 0 {
		return fmt.Errorf("endpoint %q signal_source must not set emit_signal fields", name)
	}
	if machineRequestYAMLSet(endpoint.MachineRequest) {
		return fmt.Errorf("endpoint %q signal_source must not set machine_request", name)
	}
	if lifecycleControlSet(endpoint.LifecycleControl) || queueConfigSet(endpoint.Queue) {
		return fmt.Errorf("endpoint %q signal_source must not set lifecycle_control or queue", name)
	}
	if len(endpoint.Response.Schema) > 0 || len(endpoint.Response.Output) > 0 ||
		len(endpoint.Response.Redact) > 0 || endpoint.Response.ResourceID != "" ||
		endpoint.Response.RequestID != "" {
		return fmt.Errorf("endpoint %q signal_source must use signal_source.responses", name)
	}
	return nil
}

func validateSignalSourceIdentity(name string, cfg SignalSourceBinding) error {
	if strings.TrimSpace(cfg.Source) == "" {
		return fmt.Errorf("endpoint %q signal_source requires source", name)
	}
	if cfg.Source != strings.TrimSpace(cfg.Source) {
		return fmt.Errorf("endpoint %q signal_source source must not have surrounding whitespace", name)
	}
	if len(cfg.Source) > 128 {
		return fmt.Errorf("endpoint %q signal_source source exceeds 128 bytes", name)
	}
	if len(cfg.SignalMapping) == 0 {
		return fmt.Errorf("endpoint %q signal_source requires a closed signal_mapping", name)
	}
	if len(cfg.Payload) == 0 {
		return fmt.Errorf("endpoint %q signal_source requires payload mapping", name)
	}
	return nil
}

func validateSignalSourceSelectors(
	name string,
	request RequestBinding,
	cfg SignalSourceBinding,
) error {
	if err := validateSignalSourceSelector(name, request, "discriminator_field", cfg.DiscriminatorField); err != nil {
		return err
	}
	if err := validateSignalSourceSelector(name, request, "run_id_field", cfg.RunIDField); err != nil {
		return err
	}
	if cfg.ExpectedStateField != "" {
		if err := validateSignalSourceSelector(name, request, "expected_state_field", cfg.ExpectedStateField); err != nil {
			return err
		}
	}
	return nil
}

func validateSignalSourceMappings(
	name string,
	request RequestBinding,
	cfg SignalSourceBinding,
) error {
	for discriminator, signal := range cfg.SignalMapping {
		if discriminator == "" || strings.TrimSpace(signal) == "" {
			return fmt.Errorf("endpoint %q signal_source signal_mapping keys and values must be non-empty", name)
		}
	}
	for field, selector := range cfg.Payload {
		if !validSignalPayloadField(field) {
			return fmt.Errorf("endpoint %q signal_source payload field %q is invalid", name, field)
		}
		if err := validateSignalSourceSelector(name, request, "payload."+field, selector); err != nil {
			return err
		}
	}
	return nil
}

var signalSourceAuthorityFields = map[string]bool{
	"signal": true, "profile": true, "profile_path": true, "machine": true, "method": true, "url": true, "host": true,
	"machine_spec": true, "tools": true, "tool_declarations": true, "model": true, "model_config": true, "checkpoint": true, "checkpoint_connection": true,
}

func validateSignalSourceSelector(
	name string,
	request RequestBinding,
	field string,
	selector string,
) error {
	parsed, ok := core.ParseSelector(selector)
	if !ok || parsed.Label != "" || len(parsed.Path) < 2 {
		return fmt.Errorf("endpoint %q signal_source %s %q must be a declared $.group.field selector", name, field, selector)
	}
	group, requestField := parsed.Path[0], parsed.Path[1]
	switch group {
	case "body":
		if !bodySchemaDeclares(request.BodySchema, requestField) {
			return fmt.Errorf("endpoint %q signal_source %s selects undeclared body field %q", name, field, requestField)
		}
	case "query":
		if _, ok := request.Query[requestField]; !ok || len(parsed.Path) != 2 {
			return fmt.Errorf("endpoint %q signal_source %s selects undeclared query field %q", name, field, requestField)
		}
	case "path":
		if _, ok := request.Path[requestField]; !ok || len(parsed.Path) != 2 {
			return fmt.Errorf("endpoint %q signal_source %s selects undeclared path field %q", name, field, requestField)
		}
	case "headers":
		_, declared := lookupHeaderSchema(request.Headers, requestField)
		if !declared || len(parsed.Path) != 2 || requestField != strings.ToLower(requestField) {
			return fmt.Errorf("endpoint %q signal_source %s selects undeclared header field %q", name, field, requestField)
		}
	default:
		return fmt.Errorf("endpoint %q signal_source %s selects unsupported request group %q", name, field, group)
	}
	if signalSourceAuthorityFields[strings.ToLower(requestField)] {
		return fmt.Errorf("endpoint %q signal_source %s cannot select caller program or signal authority %q", name, field, requestField)
	}
	return nil
}

func validSignalPayloadField(field string) bool {
	return field != "" && field == strings.TrimSpace(field) &&
		!strings.Contains(field, ".") && !strings.ContainsAny(field, "\r\n\t")
}

func validateSignalSourceSensitive(name string, cfg SignalSourceBinding) error {
	seen := map[string]bool{}
	for _, field := range cfg.Sensitive {
		if seen[field] {
			return fmt.Errorf("endpoint %q signal_source sensitive field %q is duplicated", name, field)
		}
		if _, ok := cfg.Payload[field]; !ok {
			return fmt.Errorf("endpoint %q signal_source sensitive field %q is not mapped payload", name, field)
		}
		seen[field] = true
	}
	return nil
}

func validateSignalSourceResponses(name string, responses SignalSourceResponseMappings) error {
	required := map[string]SignalSourceResponse{
		"accepted": responses.Accepted, "refused_undeclared": responses.RefusedUndeclared, "source_validation": responses.SourceValidation, "machine_run_failed": responses.MachineRunFailed,
	}
	for outcome, response := range required {
		if !validHTTPStatus(response.Status) {
			return fmt.Errorf("endpoint %q signal_source response %s requires HTTP status 100-599", name, outcome)
		}
	}
	if responses.RefusedConflict.Status != 0 && !validHTTPStatus(responses.RefusedConflict.Status) {
		return fmt.Errorf("endpoint %q signal_source response refused_conflict has invalid HTTP status", name)
	}
	return nil
}

func validHTTPStatus(status int) bool {
	return status >= 100 && status <= 599
}

func signalSourceYAMLSet(cfg SignalSourceBinding) bool {
	return cfg.Source != "" || cfg.DiscriminatorField != "" || len(cfg.SignalMapping) > 0 || cfg.RunIDField != "" ||
		cfg.ExpectedStateField != "" || len(cfg.Payload) > 0 ||
		len(cfg.Sensitive) > 0 || cfg.Timeout != "" || cfg.Responses.Accepted.Status != 0 ||
		cfg.Responses.RefusedUndeclared.Status != 0 || cfg.Responses.RefusedConflict.Status != 0 ||
		cfg.Responses.SourceValidation.Status != 0 || cfg.Responses.MachineRunFailed.Status != 0
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
	if len(cfg.DocumentResources) > 0 || len(cfg.Response.TerminalSignals) > 0 ||
		len(cfg.Response.TerminalStates) > 0 {
		return true
	}
	m := cfg.Request
	return len(m.Body) > 0 || len(m.Query) > 0 || len(m.Path) > 0 ||
		len(m.Headers) > 0 || len(m.Sensitive) > 0
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
