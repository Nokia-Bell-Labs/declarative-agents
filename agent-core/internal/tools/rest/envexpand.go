// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/envexpand"

// expandEnv replaces ${VAR} and ${VAR:-default} references in a REST definition
// with environment values before parsing, so one mounted profile parameterizes
// per pod (the rel06.0 mounted-profile contract). The rules live in
// support/envexpand because tool declarations expand by the same ones
// (srd013 R5.6): a set variable wins, otherwise the default is used, or empty
// when the variable is unset and no default is given. Non-brace uses of $
// (JSONPath, $from selectors) are untouched.
func expandEnv(data []byte) []byte {
	return envexpand.Expand(data)
}
