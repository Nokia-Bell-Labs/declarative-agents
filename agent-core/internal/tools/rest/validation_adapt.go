// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
	restvalidation "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/validation"
)

func validateEndpoint(name string, endpoint restdef.Endpoint) error {
	return restvalidation.ValidateEndpoint(name, endpoint)
}

func validateRetryPolicies(policies map[string]restdef.RetryPolicy) error {
	return restvalidation.ValidateRetryPolicies(policies)
}

func validateClients(
	clients map[string]restdef.Client,
	retryPolicies map[string]restdef.RetryPolicy,
	mappings map[string]restdef.ResponseMapping,
) error {
	return restvalidation.ValidateClients(clients, retryPolicies, mappings)
}

func validateMonitorView(name string, endpoint restdef.Endpoint) error {
	return restvalidation.ValidateMonitorView(name, endpoint)
}

func validateSelectorForm(name, source, selector string) error {
	return restvalidation.ValidateSelectorForm(name, source, selector)
}

func validateStatusMappings(name string, operation restdef.Operation) error {
	return restvalidation.ValidateStatusMappings(name, operation)
}

func validateLifecycleControlEndpoint(name string, endpoint restdef.Endpoint) error {
	return restvalidation.ValidateLifecycleControlEndpoint(name, endpoint)
}

func validateMachineRequestEndpoint(name string, endpoint restdef.Endpoint) error {
	return restvalidation.ValidateMachineRequestEndpoint(name, endpoint)
}

func validateMachineRequestSensitiveFields(mapping restdef.MachineRequestMapping) error {
	return restvalidation.ValidateMachineRequestSensitiveFields(mapping)
}
