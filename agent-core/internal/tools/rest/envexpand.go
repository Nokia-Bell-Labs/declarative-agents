// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"os"
	"regexp"
)

// envRefPattern matches ${NAME} and ${NAME:-default} references. It is
// brace-delimited on purpose: the pervasive $.jsonpath and $from(label)
// selectors in a REST definition carry no brace and never match, so expansion
// leaves them byte-identical.
var envRefPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:-([^}]*))?\}`)

// expandEnv replaces ${VAR} and ${VAR:-default} references in a REST definition
// with environment values before parsing, so one mounted profile parameterizes
// per pod (the rel06.0 mounted-profile contract). A set variable wins; otherwise
// the default is used, or empty when the variable is unset and no default is
// given. Non-brace uses of $ (JSONPath, $from selectors) are untouched.
func expandEnv(data []byte) []byte {
	return envRefPattern.ReplaceAllFunc(data, func(match []byte) []byte {
		groups := envRefPattern.FindSubmatch(match)
		if v, ok := os.LookupEnv(string(groups[1])); ok {
			return []byte(v)
		}
		if len(groups[2]) > 0 { // the ":-default" group is present
			return groups[3]
		}
		return nil
	})
}
