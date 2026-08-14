// Copyright (c) 2026 Nokia. All rights reserved.

package profileaudit

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolrest "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
)

const (
	defaultRESTAwait = 30 * time.Second
	defaultRESTStop  = 5 * time.Second
	defaultOTLPAwait = 30 * time.Second
	defaultOTLPRelay = 10 * time.Second
	defaultOTLPStop  = 5 * time.Second
	defaultChildRun  = 10 * time.Minute
)

func (i *inspector) inspectAction(
	closure loadedClosure,
	commandTimeout time.Duration,
	def catalog.ToolDef,
) error {
	if def.Type == "exec" || def.Type == "" {
		return nil // The dispatch timeout is the exec process authority.
	}
	for _, inspect := range []func(loadedClosure, time.Duration, catalog.ToolDef) (bool, error){
		i.inspectProfileAction,
		i.inspectRESTAction,
		i.inspectCollectorAction,
		i.inspectEvaluationAction,
	} {
		if handled, err := inspect(closure, commandTimeout, def); handled {
			return err
		}
	}
	i.inspectFallbackAction(closure, commandTimeout, def)
	return nil
}

func (i *inspector) inspectProfileAction(
	closure loadedClosure, commandTimeout time.Duration, def catalog.ToolDef,
) (bool, error) {
	switch def.Init {
	case "invoke_llm":
		i.inspectModelAuthority(closure, commandTimeout, def)
	case "delay":
		i.addConfigDuration(closure, def, "duration", commandTimeout, true)
	case "self_invoke":
		// ChildAgentConfig has no timeout field today, so execute.Config applies
		// its finite ten-minute default to every child invocation.
		i.addDuration(
			closure, def.Name, "self_invoke execute.Config timeout",
			defaultChildRun.String(), commandTimeout,
		)
		return true, i.inspectChildProfile(closure, def, "profile")
	case "run_point":
		return true, i.inspectPointMachine(closure, def)
	default:
		return false, nil
	}
	return true, nil
}

func (i *inspector) inspectModelAuthority(
	closure loadedClosure, commandTimeout time.Duration, def catalog.ToolDef,
) {
	count := 0
	for _, field := range []string{"max_time", "llm_timeout"} {
		if raw, ok := configValue(def.Config, field); ok {
			count++
			i.addSeconds(closure, def.Name, "ToolDef config."+field, raw, commandTimeout)
		}
	}
	if count == 0 {
		i.addUnsupported(closure, def, commandTimeout, "model max_time/llm_timeout")
	}
}

func (i *inspector) inspectRESTAction(
	closure loadedClosure, commandTimeout time.Duration, def catalog.ToolDef,
) (bool, error) {
	switch def.Init {
	case "rest_client_get", "rest_client_set", "rest_client_create", "rest_client_delete",
		"rest_client_invoke", "rest_client_send", "rest_client_await":
		return true, i.inspectRESTClient(closure, commandTimeout, def)
	case "rest_server_launch", "rest_server_await", "rest_server_stop":
		return true, i.inspectRESTServer(closure, commandTimeout, def)
	case "rest_await_event":
		i.inspectRESTEventAwait(closure, commandTimeout, def)
		return true, nil
	default:
		return false, nil
	}
}

func (i *inspector) inspectCollectorAction(
	closure loadedClosure, commandTimeout time.Duration, def catalog.ToolDef,
) (bool, error) {
	raw, _ := configString(def.Config, "timeout")
	switch def.Init {
	case "await_spans", "await_metrics":
		i.addDurationDefault(closure, def.Name, "ToolDef config.timeout", raw, defaultOTLPAwait, commandTimeout)
	case "relay_spans":
		i.addDurationDefault(closure, def.Name, "ToolDef config.timeout", raw, defaultOTLPRelay, commandTimeout)
	case "otlp_receiver_launch":
		// Launch only binds the listener; shutdown_timeout belongs to stop.
	case "otlp_receiver_stop":
		raw, _ := configString(def.Config, "shutdown_timeout")
		i.addDurationDefault(closure, def.Name, "ToolDef config.shutdown_timeout", raw, defaultOTLPStop, commandTimeout)
	default:
		return false, nil
	}
	return true, nil
}

func (i *inspector) inspectEvaluationAction(
	closure loadedClosure, commandTimeout time.Duration, def catalog.ToolDef,
) (bool, error) {
	switch def.Init {
	case "init_eval_session", "load_suite":
		if raw, ok := configValue(def.Config, "timeout"); ok {
			i.addSeconds(closure, def.Name, "ToolDef config.timeout", raw, commandTimeout)
		}
	case "run_scenario_validator":
		i.addConfigDuration(closure, def, "timeout", commandTimeout, true)
	case "start_scenario_mock":
		return true, i.inspectChildProfile(closure, def, "profile")
	default:
		return false, nil
	}
	return true, nil
}

func (i *inspector) inspectFallbackAction(
	closure loadedClosure, commandTimeout time.Duration, def catalog.ToolDef,
) {
	for _, field := range []string{"timeout", "await_timeout"} {
		if raw, ok := configString(def.Config, field); ok {
			i.addDuration(closure, def.Name, "ToolDef config."+field, raw, commandTimeout)
			return
		}
	}
	if unsupportedTimeoutBoundary(def) {
		i.addUnsupported(closure, def, commandTimeout, "supported finite authority mapping")
	}
}

func (i *inspector) inspectRESTEventAwait(
	closure loadedClosure, commandTimeout time.Duration, def catalog.ToolDef,
) {
	raw, _ := configString(def.Config, "timeout")
	i.addDurationDefault(closure, def.Name, "ToolDef config.timeout", raw, defaultRESTAwait, commandTimeout)
	for n, source := range configMaps(def.Config["sources"]) {
		i.inspectRESTEventSource(closure, commandTimeout, def.Name, n, source)
	}
}

func (i *inspector) inspectRESTEventSource(
	closure loadedClosure, commandTimeout time.Duration, action string,
	index int, source map[string]interface{},
) {
	server, _ := configString(source, "server")
	if raw, ok := configString(source, "timeout"); ok && raw != "" {
		i.addDuration(
			closure, action, fmt.Sprintf("ToolDef config.sources[%d].timeout", index),
			raw, commandTimeout,
		)
	}
	if resolved, err := closure.rest.ResolveServer(server); err == nil &&
		resolved.Server.Queue.Timeout != "" {
		i.addDuration(
			closure, action, fmt.Sprintf("REST server %s queue.timeout", server),
			resolved.Server.Queue.Timeout, commandTimeout,
		)
	}
}

func (i *inspector) inspectRESTClient(
	closure loadedClosure,
	commandTimeout time.Duration,
	def catalog.ToolDef,
) error {
	restRef, _ := configString(def.Config, "rest_ref")
	resource, _ := configString(def.Config, "resource")
	operation, _ := configString(def.Config, "operation")
	resolved, err := closure.rest.ResolveClientOperation(toolrest.ClientToolConfig{
		RestRef: restRef, Resource: resource, Operation: operation,
	})
	if err != nil {
		return fmt.Errorf("profile %s action %q REST authority: %w", closure.profilePath, def.Name, err)
	}
	base := fmt.Sprintf("REST client %s limits", restRef)
	if resolved.Limits.Timeout == "" {
		i.addUnsupported(closure, def, commandTimeout, base+".timeout")
	} else {
		i.addDuration(closure, def.Name, base+".timeout", resolved.Limits.Timeout, commandTimeout)
	}
	for field, raw := range map[string]string{
		"connect_timeout": resolved.Limits.ConnectTimeout,
		"read_timeout":    resolved.Limits.ReadTimeout,
	} {
		if raw != "" {
			i.addDuration(closure, def.Name, base+"."+field, raw, commandTimeout)
		}
	}
	if def.Init == "rest_client_send" || def.Init == "rest_client_await" {
		if resolved.Operation.Async == nil {
			return fmt.Errorf("profile %s action %q requires an async REST operation", closure.profilePath, def.Name)
		}
		i.addDurationDefault(
			closure, def.Name,
			fmt.Sprintf("REST client %s operation %s async.timeout", restRef, operation),
			resolved.Operation.Async.Timeout, defaultRESTAwait, commandTimeout,
		)
	}
	return nil
}

func (i *inspector) inspectRESTServer(
	closure loadedClosure,
	commandTimeout time.Duration,
	def catalog.ToolDef,
) error {
	restRef, _ := configString(def.Config, "rest_ref")
	resolved, err := closure.rest.ResolveServer(restRef)
	if err != nil {
		return fmt.Errorf("profile %s action %q REST authority: %w", closure.profilePath, def.Name, err)
	}
	switch def.Init {
	case "rest_server_await":
		i.addDurationDefault(
			closure, def.Name, fmt.Sprintf("REST server %s queue.timeout", restRef),
			resolved.Server.Queue.Timeout, defaultRESTAwait, commandTimeout,
		)
	case "rest_server_stop":
		i.addDurationDefault(
			closure, def.Name, fmt.Sprintf("REST server %s shutdown.timeout", restRef),
			resolved.Server.Shutdown.Timeout, defaultRESTStop, commandTimeout,
		)
	case "rest_server_launch":
		return i.inspectRESTServerLaunch(closure, commandTimeout, def, resolved)
	}
	return nil
}

func (i *inspector) inspectRESTServerLaunch(
	closure loadedClosure, commandTimeout time.Duration,
	def catalog.ToolDef, server toolrest.ServerDefinition,
) error {
	names := make([]string, 0, len(server.Server.Endpoints))
	for name := range server.Server.Endpoints {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		endpoint := server.Server.Endpoints[name]
		if endpoint.Binding != "machine_request" {
			continue
		}
		if err := i.inspectMachineRequestEndpoint(
			closure, commandTimeout, def.Name, server, name, endpoint,
		); err != nil {
			return err
		}
	}
	return nil
}

func (i *inspector) inspectMachineRequestEndpoint(
	closure loadedClosure, commandTimeout time.Duration, action string,
	server toolrest.ServerDefinition, name string, endpoint toolrest.Endpoint,
) error {
	raw := endpoint.MachineRequest.Timeout
	if raw == "" {
		raw = server.Limits.Timeout
	}
	i.addDurationDefault(
		closure, action,
		fmt.Sprintf("REST server %s endpoint %s machine_request.timeout", server.Name, name),
		raw, defaultRESTAwait, commandTimeout,
	)
	if endpoint.MachineRequest.Profile == "" {
		return fmt.Errorf("profile %s REST endpoint %s/%s has no machine_request profile",
			closure.profilePath, server.Name, name)
	}
	profilePath := resolveReference(
		filepath.Dir(closure.profilePath), endpoint.MachineRequest.Profile,
	)
	return i.inspectProfile(profilePath, endpoint.MachineRequest.Machine)
}

func (i *inspector) addConfigDuration(
	closure loadedClosure,
	def catalog.ToolDef,
	field string,
	commandTimeout time.Duration,
	required bool,
) {
	raw, ok := configString(def.Config, field)
	if !ok || raw == "" {
		if required {
			i.addUnsupported(closure, def, commandTimeout, "ToolDef config."+field)
		}
		return
	}
	i.addDuration(closure, def.Name, "ToolDef config."+field, raw, commandTimeout)
}

func (i *inspector) addDurationDefault(
	closure loadedClosure,
	action, source, raw string,
	fallback, commandTimeout time.Duration,
) {
	if raw == "" {
		raw = fallback.String()
	}
	i.addDuration(closure, action, source, raw, commandTimeout)
}

func (i *inspector) addDuration(
	closure loadedClosure,
	action, source, raw string,
	commandTimeout time.Duration,
) {
	duration, err := time.ParseDuration(raw)
	operation := Operation{
		Profile: closure.profilePath, Machine: closure.machinePath, Action: action,
		Authority: source, RawDuration: raw, Duration: duration, CommandTimeout: commandTimeout,
	}
	i.operations = append(i.operations, operation)
	switch {
	case err != nil:
		i.diagnostics = append(i.diagnostics, Diagnostic{Operation: operation, Reason: "operation duration is malformed"})
	case duration <= 0:
		i.diagnostics = append(i.diagnostics, Diagnostic{Operation: operation, Reason: "operation duration must be positive"})
	case duration >= commandTimeout:
		i.diagnostics = append(i.diagnostics, Diagnostic{
			Operation: operation,
			Reason:    "operation duration must be strictly below command_timeout",
		})
	}
}

func (i *inspector) addSeconds(
	closure loadedClosure,
	action, source string,
	value interface{},
	commandTimeout time.Duration,
) {
	seconds, raw, ok := positiveSeconds(value)
	if !ok {
		i.addInvalid(closure, action, source, raw, commandTimeout,
			"operation duration must be a positive whole number of seconds")
		return
	}
	i.addDuration(closure, action, source, (time.Duration(seconds) * time.Second).String(), commandTimeout)
}

func (i *inspector) addUnsupported(
	closure loadedClosure,
	def catalog.ToolDef,
	commandTimeout time.Duration,
	source string,
) {
	i.addInvalid(closure, def.Name, source, "unresolved", commandTimeout,
		"reachable timeout-bearing boundary has no supported finite authority")
}

func (i *inspector) addInvalid(
	closure loadedClosure,
	action, source, raw string,
	commandTimeout time.Duration,
	reason string,
) {
	operation := Operation{
		Profile: closure.profilePath, Machine: closure.machinePath, Action: action,
		Authority: source, RawDuration: raw, CommandTimeout: commandTimeout,
	}
	i.operations = append(i.operations, operation)
	i.diagnostics = append(i.diagnostics, Diagnostic{Operation: operation, Reason: reason})
}

func unsupportedTimeoutBoundary(def catalog.ToolDef) bool {
	if hasTimeoutLikeField(def.Config) {
		return true
	}
	if def.Category != "boundary" {
		return false
	}
	init := strings.ToLower(def.Init)
	return strings.Contains(init, "await") || strings.Contains(init, "delay") ||
		strings.Contains(init, "timeout")
}

func hasTimeoutLikeField(config map[string]interface{}) bool {
	for key, value := range config {
		normalized := strings.ToLower(key)
		if strings.Contains(normalized, "timeout") || normalized == "duration" ||
			normalized == "max_time" {
			return true
		}
		if nested, ok := value.(map[string]interface{}); ok && hasTimeoutLikeField(nested) {
			return true
		}
	}
	return false
}

func configValue(config map[string]interface{}, key string) (interface{}, bool) {
	value, ok := config[key]
	return value, ok
}

func configString(config map[string]interface{}, key string) (string, bool) {
	value, ok := config[key]
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	return text, ok
}

func configStrings(value interface{}) ([]string, bool) {
	switch values := value.(type) {
	case []interface{}:
		out := make([]string, 0, len(values))
		for _, value := range values {
			text, ok := value.(string)
			if !ok {
				return nil, false
			}
			out = append(out, text)
		}
		return out, len(out) > 0
	case []string:
		return append([]string(nil), values...), len(values) > 0
	default:
		return nil, false
	}
}

func configMaps(value interface{}) []map[string]interface{} {
	values, _ := value.([]interface{})
	out := make([]map[string]interface{}, 0, len(values))
	for _, value := range values {
		if entry, ok := value.(map[string]interface{}); ok {
			out = append(out, entry)
		}
	}
	return out
}

func positiveSeconds(value interface{}) (int64, string, bool) {
	raw := fmt.Sprint(value)
	switch number := value.(type) {
	case int:
		return int64(number), raw, number > 0
	case int64:
		return number, raw, number > 0
	case float64:
		if number > 0 && number == float64(int64(number)) {
			return int64(number), raw, true
		}
	case string:
		parsed, err := strconv.ParseInt(number, 10, 64)
		return parsed, number, err == nil && parsed > 0
	}
	return 0, raw, false
}
