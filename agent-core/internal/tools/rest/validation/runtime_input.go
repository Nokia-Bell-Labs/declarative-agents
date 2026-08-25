// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package validation

import restclient "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/client"

// ValidateRuntimeInput rejects transport authority supplied at runtime.
func ValidateRuntimeInput(input map[string]interface{}) error {
	return restclient.ValidateRuntimeInput(input)
}
