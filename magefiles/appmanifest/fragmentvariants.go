// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package appmanifest

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// envTemplatePattern is agent-core's envexpand grammar: ${NAME} and
// ${NAME:-default}. A tool-declarations file is expanded by it before decoding,
// so a fragment path may select a variant at deploy time (GH-2232).
var envTemplatePattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:-([^}]*))?\}`)

// templateTokenPattern matches the recoverable stand-in yamlTemplateTokens
// writes for each template, so a templated fragment path survives parsing.
var templateTokenPattern = regexp.MustCompile(`manifest_value_t([0-9]+)_`)

// closureTemplate is one ${...} template of a closure source.
type closureTemplate struct {
	text       string
	name       string
	hasDefault bool
	defaultVal string
}

// yamlTemplateTokens makes data parseable like yamlTemplateSafe, but writes a
// numbered token per template so a fragment path can be mapped back to the
// templates it carried. A template outside the envexpand grammar is recorded
// without a name.
func yamlTemplateTokens(data []byte) ([]byte, []closureTemplate) {
	var templates []closureTemplate
	safe := templatePattern.ReplaceAllFunc(data, func(match []byte) []byte {
		template := closureTemplate{text: string(match)}
		if groups := envTemplatePattern.FindSubmatch(match); groups != nil && len(groups[0]) == len(match) {
			template.name = string(groups[1])
			template.hasDefault = len(groups[2]) > 0
			template.defaultVal = string(groups[3])
		}
		templates = append(templates, template)
		return []byte("manifest_value_t" + strconv.Itoa(len(templates)-1) + "_")
	})
	return safe, templates
}

// untokenized restores today's reference text for a non-fragment reference:
// every template reads manifest_value, as yamlTemplateSafe writes it.
func untokenized(reference string) string {
	return templateTokenPattern.ReplaceAllString(reference, "manifest_value")
}

// fragmentVariants expands an expand -> fragment path whose templates
// select a variant at deploy time into every file in the owning root it can
// name, because the binding is chosen where the chart is installed. Each
// template becomes a single-segment wildcard. It fails, naming the path, when a
// template is not ${NAME:-default}, when nothing matches, or when the variant
// the defaults select is not among the matches.
func (resolver *closureResolver) fragmentVariants(
	item closureItem, reference string, templates []closureTemplate,
) ([]string, error) {
	origin := logicalSource(item.ownership, item.source)
	var used []closureTemplate
	for _, match := range templateTokenPattern.FindAllStringSubmatch(reference, -1) {
		index, _ := strconv.Atoi(match[1])
		if index >= len(templates) {
			return nil, fmt.Errorf("%s: unmatched template token in fragment %s", origin, reference)
		}
		template := templates[index]
		if template.name == "" || !template.hasDefault {
			return nil, fmt.Errorf("%s expands templated fragment %s: %s needs the form ${NAME:-default} "+
				"so the chart can name the variant a deployment gets when NAME is unset",
				origin, restoreTemplates(reference, templates), template.text)
		}
		used = append(used, template)
	}
	written := restoreTemplates(reference, templates)
	defaultVariant := templateTokenPattern.ReplaceAllStringFunc(reference, func(token string) string {
		index, _ := strconv.Atoi(templateTokenPattern.FindStringSubmatch(token)[1])
		return templates[index].defaultVal
	})
	if strings.ContainsAny(defaultVariant, "*?[") || strings.ContainsAny(untokenized(reference), "*?[") {
		return nil, fmt.Errorf("%s contains unbounded glob reference %s", origin, written)
	}
	// Resolve the default variant for its ownership and source directory; the
	// wildcard pattern resolves the same way, since only template segments differ.
	wildcard := templateTokenPattern.ReplaceAllString(reference, "\x00")
	ownership, sourcePattern, err := resolver.variantSourcePattern(item, reference, wildcard)
	if err != nil {
		return nil, fmt.Errorf("%s expands templated fragment %s: %w", origin, written, err)
	}
	root := resolver.applicationRoot
	if ownership == "catalog" {
		root = resolver.catalogRoot
	}
	globPattern := strings.ReplaceAll(sourcePattern, "\x00", "*")
	matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(globPattern)))
	if err != nil {
		return nil, fmt.Errorf("%s expands templated fragment %s: %w", origin, written, err)
	}
	capture := regexp.MustCompile("^" + strings.ReplaceAll(regexp.QuoteMeta(sourcePattern), "\x00", "([^/]*)") + "$")
	var variants []string
	for _, match := range matches {
		relative, err := filepath.Rel(root, match)
		if err != nil {
			continue
		}
		groups := capture.FindStringSubmatch(filepath.ToSlash(relative))
		if groups == nil || len(groups)-1 != len(used) {
			continue
		}
		next := 1
		variants = append(variants, templateTokenPattern.ReplaceAllStringFunc(reference, func(string) string {
			value := groups[next]
			next++
			return value
		}))
	}
	sort.Strings(variants)
	if len(variants) == 0 {
		return nil, fmt.Errorf("%s expands templated fragment %s: no file matches %s",
			origin, written, globPattern)
	}
	if !contains(variants, defaultVariant) {
		return nil, fmt.Errorf("%s expands templated fragment %s: the default variant %s is missing "+
			"(found %s)", origin, written, defaultVariant, strings.Join(variants, ", "))
	}
	return variants, nil
}

// variantSourcePattern resolves a templated reference to its owning root and
// a source-relative pattern with a NUL where each template stood.
func (resolver *closureResolver) variantSourcePattern(
	item closureItem, reference, wildcard string,
) (string, string, error) {
	resolveFrom, relative := item, wildcard
	if name, rest, rooted := libraryReference(wildcard); rooted {
		library, declared := resolver.libraries[item.rootID][name]
		if !declared {
			return "", "", fmt.Errorf("library root %q is not declared by the profile", name)
		}
		resolveFrom, relative = library.profile, path.Join(library.directory, rest)
	} else if path.IsAbs(filepath.ToSlash(reference)) || isWindowsPath(reference) {
		return "", "", fmt.Errorf("disallowed absolute reference")
	}
	ownership, source, _, _, err := resolver.resolveReference(resolveFrom, relative)
	if err != nil {
		return "", "", err
	}
	if strings.Count(source, "\x00") != len(templateTokenPattern.FindAllString(reference, -1)) {
		return "", "", fmt.Errorf("template segments do not survive path resolution")
	}
	return ownership, source, nil
}

// restoreTemplates writes a tokenized reference back as the author wrote it.
func restoreTemplates(reference string, templates []closureTemplate) string {
	return templateTokenPattern.ReplaceAllStringFunc(reference, func(token string) string {
		index, _ := strconv.Atoi(templateTokenPattern.FindStringSubmatch(token)[1])
		if index < len(templates) {
			return templates[index].text
		}
		return token
	})
}
