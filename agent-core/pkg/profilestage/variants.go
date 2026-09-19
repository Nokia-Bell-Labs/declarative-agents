// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package profilestage

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/envexpand"
)

// variantEdges expands an edge that selects a variant by environment
// (srd052 R4.3), such as stages/rerank-${PROVIDER:-none}.yaml, into every
// variant file beside it: the binding is chosen where the profile runs, so a
// staged tree carries all of them. Every other edge passes through unchanged.
// base is the directory a relative edge resolves against; libraryDir maps a
// declared library root to its directory. A variant edge without a default, or
// whose default file is absent, is an error naming the edge.
func variantEdges(edge, base string, libraryDir func(string) (string, bool)) ([]string, error) {
	if !envexpand.Templated(edge) {
		return []string{edge}, nil
	}
	variant, err := envexpand.SelectVariant(edge)
	if err != nil {
		return nil, fmt.Errorf("fragment %q: %w", edge, err)
	}
	sourcePattern, expand := variantSourcePattern(edge, variant.Pattern, base, libraryDir)
	if !expand {
		return []string{edge}, nil
	}
	edges, err := matchedEdges(sourcePattern, variant.Pattern)
	if err != nil {
		return nil, fmt.Errorf("fragment %q: %w", edge, err)
	}
	if !contains(edges, variant.Default) {
		return nil, fmt.Errorf("fragment %q: default variant %s is missing (found %s)",
			edge, variant.Default, strings.Join(edges, ", "))
	}
	return edges, nil
}

// variantSourcePattern places a variant pattern in the source tree: relative
// to base, or under a declared library root. It declines agent-core's library,
// which the image supplies, and an undeclared root, which the caller reports.
func variantSourcePattern(edge, pattern, base string, libraryDir func(string) (string, bool)) (string, bool) {
	if !filepath.IsAbs(edge) {
		return filepath.ToSlash(filepath.Join(base, pattern)), true
	}
	root, rest, ok := libraryReference(pattern)
	directory, declared := libraryDir(root)
	if isAgentCoreLibraryPath(edge) || !ok || !declared {
		return "", false
	}
	return filepath.ToSlash(filepath.Join(directory, rest)), true
}

// matchedEdges globs sourcePattern and writes each match back into the edge's
// own form by filling the pattern's wildcards with what each matched.
func matchedEdges(sourcePattern, pattern string) ([]string, error) {
	matches, err := filepath.Glob(filepath.FromSlash(sourcePattern))
	if err != nil {
		return nil, err
	}
	capture := regexp.MustCompile("^" + strings.ReplaceAll(regexp.QuoteMeta(sourcePattern), `\*`, "([^/]*)") + "$")
	var edges []string
	for _, match := range matches {
		groups := capture.FindStringSubmatch(filepath.ToSlash(match))
		if groups == nil {
			continue
		}
		next := 1
		edges = append(edges, wildcard.ReplaceAllStringFunc(pattern, func(string) string {
			value := groups[next]
			next++
			return value
		}))
	}
	sort.Strings(edges)
	return edges, nil
}

var wildcard = regexp.MustCompile(`\*`)

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
