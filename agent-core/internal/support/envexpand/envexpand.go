// Copyright (c) 2026 Nokia. All rights reserved.

// Package envexpand expands environment references in mounted configuration
// before it is parsed. REST definitions and tool declarations both use it, so a
// single mounted profile parameterizes per pod by the same rules whichever form
// carries the address (srd013 R5.6, srd028 R2).
package envexpand

import (
	"os"
	"regexp"
)

// refPattern matches ${NAME} and ${NAME:-default} references. It is
// brace-delimited on purpose: the pervasive $.jsonpath and $from(label)
// selectors in a configuration file carry no brace and never match, so
// expansion leaves them byte-identical.
var refPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:-([^}]*))?\}`)

// Expand replaces ${VAR} and ${VAR:-default} references with environment
// values. A set variable wins; otherwise the default is used, or empty when the
// variable is unset and no default is given.
func Expand(data []byte) []byte {
	return refPattern.ReplaceAllFunc(data, func(match []byte) []byte {
		groups := refPattern.FindSubmatch(match)
		if v, ok := os.LookupEnv(string(groups[1])); ok {
			return []byte(v)
		}
		if len(groups[2]) > 0 { // the ":-default" group is present
			return groups[3]
		}
		return nil
	})
}
