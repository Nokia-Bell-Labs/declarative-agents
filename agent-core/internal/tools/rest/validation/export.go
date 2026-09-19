// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package validation

// Exported entry points used by parent tests that still live in package rest.

func ValidateEndpoint(name string, endpoint Endpoint) error {
	return validateEndpoint(name, endpoint)
}

func ValidateRetryPolicies(policies map[string]RetryPolicy) error {
	return validateRetryPolicies(policies)
}

func ValidateClients(clients map[string]Client, retryPolicies map[string]RetryPolicy, mappings map[string]ResponseMapping) error {
	return validateClients(clients, retryPolicies, mappings)
}

func ValidateMonitorView(name string, endpoint Endpoint) error {
	return validateMonitorView(name, endpoint)
}

func ValidateSelectorForm(name, source, selector string) error {
	return validateSelectorForm(name, source, selector)
}

func ValidateStatusMappings(name string, operation Operation) error {
	return validateStatusMappings(name, operation)
}

func ValidateLifecycleControlEndpoint(name string, endpoint Endpoint) error {
	return validateLifecycleControlEndpoint(name, endpoint)
}

func ValidateMachineRequestEndpoint(name string, endpoint Endpoint) error {
	return validateMachineRequestEndpoint(name, endpoint)
}

func ValidateMachineRequestSensitiveFields(mapping MachineRequestMapping) error {
	return validateMachineRequestSensitiveFields(mapping)
}

// ValidateAuthProfile checks one auth profile the way a REST definition's auth
// map is checked, for a declaration that carries a single profile (srd058 R2.1).
func ValidateAuthProfile(name string, profile AuthProfile) error {
	return validateAuthProfiles(map[string]AuthProfile{name: profile})
}
