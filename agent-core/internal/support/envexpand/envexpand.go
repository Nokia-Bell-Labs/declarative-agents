// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package envexpand expands environment references in mounted configuration
// before it is parsed. REST definitions and tool declarations both use it, so a
// single mounted profile parameterizes per pod by the same rules whichever form
// carries the address (srd013 R5.6, srd028 R2).
package envexpand

import (
	"fmt"
	"os"
	"regexp"
	"strings"
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
		//nolint:forbidigo // srd013 R5.6/R5.7 requires ${VAR:-default} expansion at the declaration boundary.
		if v, ok := os.LookupEnv(string(groups[1])); ok {
			return []byte(v)
		}
		if len(groups[2]) > 0 { // the ":-default" group is present
			return groups[3]
		}
		return nil
	})
}

// Templated reports whether s carries a ${...} reference, well-formed or not.
func Templated(s string) bool {
	return strings.Contains(s, "${")
}

// Variant is a path that selects among files by environment: every reference
// is ${NAME:-default}, so the default names the file a deployment gets when
// NAME is unset, and Pattern is the path with each reference as a wildcard.
type Variant struct {
	Default string
	Pattern string
}

// SelectVariant reads a path whose references select a variant (srd052 R4.3).
// A reference without a default, or a ${ outside the reference grammar, is an
// error: the default is what names the variant every deployment can rely on.
func SelectVariant(path string) (Variant, error) {
	matches := refPattern.FindAllStringSubmatch(path, -1)
	for _, match := range matches {
		if match[2] == "" {
			return Variant{}, fmt.Errorf("reference %s needs the form ${NAME:-default}", match[0])
		}
	}
	if strings.Contains(refPattern.ReplaceAllString(path, ""), "${") {
		return Variant{}, fmt.Errorf("reference in %q is not ${NAME:-default}", path)
	}
	return Variant{
		Default: refPattern.ReplaceAllString(path, "${3}"),
		Pattern: refPattern.ReplaceAllString(path, "*"),
	}, nil
}

// ExpandString is Expand for one string.
func ExpandString(s string) string {
	return string(Expand([]byte(s)))
}
