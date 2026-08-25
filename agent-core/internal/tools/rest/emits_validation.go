// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

func validateClientEmits(def catalog.ToolDef, init string, operation ClientOperationDefinition) error {
	actual := []string{"CommandError"}
	switch init {
	case InitClientSend:
		actual = append(actual, "RESTAccepted")
	case InitClientAwait:
		actual = append(actual, operationSignals(operation.Operation)...)
		actual = append(actual, "RESTAwaitTimedOut")
	default:
		actual = append(actual, operationSignals(operation.Operation)...)
	}
	return requireDeclaredSignals(def, actual, "REST client operation "+operation.OperationName)
}

func validateServerAwaitEmits(def catalog.ToolDef, server ServerDefinition) error {
	actual := append([]string{
		"AwaitTimedOut", shutdownUnblockSignal(server.Server.Shutdown), "CommandError",
	},
		serverEndpointSignals(server.Server)...)
	return requireDeclaredSignals(def, actual, "REST server "+server.Name)
}

func validateAwaitAnyEmits(
	def catalog.ToolDef, options AwaitAnyOptions, definitions Collection,
) error {
	actual := []string{"AwaitTimedOut", "CommandError"}
	for _, source := range options.Sources {
		server, err := definitions.ResolveServer(source.Server)
		if err != nil {
			return err
		}
		if stoppedBehavior(source.StoppedBehavior, options.StoppedBehavior) ==
			StoppedSourceEmitServerStopped {
			actual = append(actual, shutdownUnblockSignal(server.Server.Shutdown))
		}
		if len(source.Signals) > 0 {
			actual = append(actual, source.Signals...)
			continue
		}
		actual = append(actual, awaitSourceSignals(server.Server, source.Routes)...)
	}
	return requireDeclaredSignals(def, actual, "REST event sources")
}

func awaitSourceSignals(server Server, routes []string) []string {
	if len(routes) == 0 {
		return serverEndpointSignals(server)
	}
	selected := make(map[string]bool, len(routes))
	for _, route := range routes {
		selected[route] = true
	}
	endpoints := injectLifecycleExit(server)
	var signals []string
	for name, endpoint := range endpoints {
		if selected[name] {
			signals = append(signals, endpointSignals(endpoint)...)
		}
	}
	return signals
}

func operationSignals(operation Operation) []string {
	signals := []string{operation.Success.Signal}
	for _, mapping := range operation.Failures {
		signals = append(signals, mapping.Signal)
	}
	return signals
}

func serverEndpointSignals(server Server) []string {
	endpoints := injectLifecycleExit(server)
	var signals []string
	for _, endpoint := range endpoints {
		signals = append(signals, endpointSignals(endpoint)...)
	}
	return signals
}

// ServerEndpointSignals returns the grammar signals a server's endpoints may emit.
func ServerEndpointSignals(server Server) []string {
	return serverEndpointSignals(server)
}

func endpointSignals(endpoint Endpoint) []string {
	signals := []string{lifecycleSignal(endpoint)}
	signals = append(signals, endpoint.AllowedSignals...)
	for _, signal := range endpoint.SignalMapping {
		signals = append(signals, signal)
	}
	return signals
}

func requireDeclaredSignals(def catalog.ToolDef, actual []string, owner string) error {
	declared := make(map[string]bool, len(def.Emits))
	for _, signal := range def.Emits {
		declared[signal] = true
	}
	var missing []string
	for _, signal := range actual {
		if signal != "" && !declared[signal] {
			missing = appendOnce(missing, signal)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf(
		"tool %q %s emits undeclared signal(s) %s; declared emits: %s",
		def.Name, owner, strings.Join(missing, ", "), strings.Join(def.Emits, ", "),
	)
}

func appendOnce(values []string, value string) []string {
	for _, current := range values {
		if current == value {
			return values
		}
	}
	return append(values, value)
}
