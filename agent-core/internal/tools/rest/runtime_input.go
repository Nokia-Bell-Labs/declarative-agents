// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import restclient "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/client"

var forbiddenRuntimeAuthorityFields = map[string]bool{
	"auth":            true,
	"auth_ref":        true,
	"base_url":        true,
	"host":            true,
	"method":          true,
	"redirect":        true,
	"redirect_policy": true,
	"url":             true,
}

// ValidateRuntimeInput rejects transport authority supplied at runtime.
func ValidateRuntimeInput(input map[string]interface{}) error {
	return restclient.ValidateRuntimeInput(input)
}

// declaredParamNames is the set of param names an operation declares across its
// path, query, header, and body-schema bindings.
func declaredParamNames(binding RequestBinding) map[string]bool {
	return restclient.DeclaredParamNames(binding)
}
