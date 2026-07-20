// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ExecuteConsistencyChecks runs consistency_check charter checks over targetDir.
func ExecuteConsistencyChecks(targetDir string, charters []Charter) ([]Finding, error) {
	var findings []Finding
	for _, charter := range charters {
		root, rootRel := charterRoot(targetDir, charter.Target.Root)
		files, err := charterFiles(root, rootRel, charter.Target.Include, charter.Target.Exclude)
		if err != nil {
			return nil, fmt.Errorf("charter %q: %w", charter.ID, err)
		}
		for _, check := range charter.Checks {
			if check.Kind != "consistency_check" {
				continue
			}
			checkFindings, err := executeConsistencyCheck(targetDir, charter, check, root, rootRel, files)
			if err != nil {
				return nil, err
			}
			findings = append(findings, checkFindings...)
		}
	}
	sort.Slice(findings, func(i, j int) bool {
		return findingLess(findings[i], findings[j])
	})
	return findings, nil
}

func executeConsistencyCheck(targetDir string, charter Charter, check CharterCheck, root, rootRel string, baseFiles []charterFile) ([]Finding, error) {
	files, err := consistencySourceFiles(root, rootRel, baseFiles, check)
	if err != nil {
		return nil, fmt.Errorf("charter %q check %q: %w", charter.ID, check.ID, err)
	}
	var findings []Finding
	for _, file := range files {
		doc, err := readYAMLDocument(file.abs)
		if err != nil {
			return nil, fmt.Errorf("charter %q check %q: %w", charter.ID, check.ID, err)
		}
		sourceValues, err := yamlPathValues(doc, sourceYAMLPath(check))
		if err != nil {
			return nil, fmt.Errorf("charter %q check %q: %w", charter.ID, check.ID, err)
		}
		switch check.Rule {
		case "equals":
			ruleFindings, err := consistencyEqualsFindings(charter, check, file, doc, sourceValues)
			if err != nil {
				return nil, err
			}
			findings = append(findings, ruleFindings...)
		case "required_path_exists":
			findings = append(findings, consistencyPathFindings(targetDir, root, charter, check, file, sourceValues)...)
		case "required_when":
			ruleFindings, err := consistencyRequiredWhenFindings(targetDir, root, charter, check, file, doc, sourceValues)
			if err != nil {
				return nil, err
			}
			findings = append(findings, ruleFindings...)
		default:
			return nil, fmt.Errorf("charter %q check %q: unknown consistency_check rule %q", charter.ID, check.ID, check.Rule)
		}
	}
	return findings, nil
}

func consistencySourceFiles(root, rootRel string, baseFiles []charterFile, check CharterCheck) ([]charterFile, error) {
	if file, ok := stringMapValue(check.Source, "file"); ok && file != "" {
		abs := resolveCharterPath("", root, file)
		rel := filepath.ToSlash(file)
		return []charterFile{{abs: abs, rel: rel, display: displayCharterPath(rootRel, rel)}}, nil
	}
	return narrowCharterFiles(root, rootRel, baseFiles, check.Include, check.Exclude)
}

func readYAMLDocument(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read YAML file %s: %w", path, err)
	}
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, fmt.Errorf("parse YAML file %s: %w", path, err)
	}
	if len(node.Content) == 0 {
		return &node, nil
	}
	return node.Content[0], nil
}

type yamlSelectedValue struct {
	value string
	line  int
}

func sourceYAMLPath(check CharterCheck) string {
	path, _ := stringMapValue(check.Source, "yaml_path")
	return path
}

func targetYAMLPath(check CharterCheck) string {
	path, _ := stringMapValue(check.Target, "yaml_path")
	return path
}

func yamlPathValues(root *yaml.Node, path string) ([]yamlSelectedValue, error) {
	if path == "" {
		return nil, fmt.Errorf("source yaml_path is required")
	}
	segments, err := parseYAMLPath(path)
	if err != nil {
		return nil, err
	}
	nodes := []*yaml.Node{root}
	for _, segment := range segments {
		var next []*yaml.Node
		for _, node := range nodes {
			next = append(next, selectYAMLPathSegment(node, segment)...)
		}
		nodes = next
	}
	values := make([]yamlSelectedValue, 0, len(nodes))
	for _, node := range nodes {
		values = append(values, yamlSelectedValue{value: node.Value, line: node.Line})
	}
	return values, nil
}

type yamlPathSegment struct {
	key      string
	wildcard bool
	filter   *yamlPathFilter
}

// yamlPathFilter selects sequence items whose mapping field matches value.
// negate inverts the match (field != value).
type yamlPathFilter struct {
	field  string
	value  string
	negate bool
}

func parseYAMLPath(path string) ([]yamlPathSegment, error) {
	if !strings.HasPrefix(path, "$.") {
		return nil, fmt.Errorf("yaml_path %q must use the $. prefix", path)
	}
	rawSegments, err := splitPathSegments(strings.TrimPrefix(path, "$."))
	if err != nil {
		return nil, fmt.Errorf("yaml_path %q: %w", path, err)
	}
	segments := make([]yamlPathSegment, 0, len(rawSegments))
	for _, raw := range rawSegments {
		if raw == "" {
			return nil, fmt.Errorf("yaml_path %q contains an empty segment", path)
		}
		segment, err := parsePathSegment(raw)
		if err != nil {
			return nil, fmt.Errorf("yaml_path %q: %w", path, err)
		}
		segments = append(segments, segment)
	}
	return segments, nil
}

// splitPathSegments splits a path on "." but not inside a "[...]" bracket, so
// filter predicates such as [?status=done] survive the split intact.
func splitPathSegments(path string) ([]string, error) {
	var segments []string
	var current strings.Builder
	depth := 0
	for _, r := range path {
		switch r {
		case '[':
			depth++
			current.WriteRune(r)
		case ']':
			depth--
			if depth < 0 {
				return nil, fmt.Errorf("unbalanced ] in segment")
			}
			current.WriteRune(r)
		case '.':
			if depth == 0 {
				segments = append(segments, current.String())
				current.Reset()
				continue
			}
			current.WriteRune(r)
		default:
			current.WriteRune(r)
		}
	}
	if depth != 0 {
		return nil, fmt.Errorf("unbalanced [ in segment")
	}
	segments = append(segments, current.String())
	return segments, nil
}

func parsePathSegment(raw string) (yamlPathSegment, error) {
	if strings.HasSuffix(raw, "[*]") {
		return yamlPathSegment{key: strings.TrimSuffix(raw, "[*]"), wildcard: true}, nil
	}
	open := strings.Index(raw, "[?")
	if open >= 0 {
		if !strings.HasSuffix(raw, "]") {
			return yamlPathSegment{}, fmt.Errorf("filter segment %q must end with ]", raw)
		}
		predicate := raw[open+2 : len(raw)-1]
		filter, err := parsePathFilter(predicate)
		if err != nil {
			return yamlPathSegment{}, fmt.Errorf("filter segment %q: %w", raw, err)
		}
		return yamlPathSegment{key: raw[:open], filter: filter}, nil
	}
	return yamlPathSegment{key: raw}, nil
}

func parsePathFilter(predicate string) (*yamlPathFilter, error) {
	op := "="
	if idx := strings.Index(predicate, "!="); idx >= 0 {
		field := strings.TrimSpace(predicate[:idx])
		value := strings.TrimSpace(predicate[idx+2:])
		if field == "" {
			return nil, fmt.Errorf("filter %q has an empty field", predicate)
		}
		return &yamlPathFilter{field: field, value: value, negate: true}, nil
	}
	idx := strings.Index(predicate, op)
	if idx < 0 {
		return nil, fmt.Errorf("filter %q must be field=value or field!=value", predicate)
	}
	field := strings.TrimSpace(predicate[:idx])
	value := strings.TrimSpace(predicate[idx+1:])
	if field == "" {
		return nil, fmt.Errorf("filter %q has an empty field", predicate)
	}
	return &yamlPathFilter{field: field, value: value}, nil
}

func selectYAMLPathSegment(node *yaml.Node, segment yamlPathSegment) []*yaml.Node {
	var selected []*yaml.Node
	if segment.key == "" {
		selected = append(selected, node)
	} else if node.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == segment.key {
				selected = append(selected, node.Content[i+1])
				break
			}
		}
	}
	if segment.filter != nil {
		return filterSequenceItems(selected, segment.filter)
	}
	if !segment.wildcard {
		return selected
	}
	var expanded []*yaml.Node
	for _, item := range selected {
		if item.Kind == yaml.SequenceNode {
			expanded = append(expanded, item.Content...)
		}
	}
	return expanded
}

// filterSequenceItems expands sequence nodes to the items whose mapping field
// satisfies the filter predicate.
func filterSequenceItems(nodes []*yaml.Node, filter *yamlPathFilter) []*yaml.Node {
	var matched []*yaml.Node
	for _, node := range nodes {
		if node.Kind != yaml.SequenceNode {
			continue
		}
		for _, item := range node.Content {
			if item.Kind != yaml.MappingNode {
				continue
			}
			value, ok := mappingFieldValue(item, filter.field)
			if !ok {
				continue
			}
			if (value == filter.value) != filter.negate {
				matched = append(matched, item)
			}
		}
	}
	return matched
}

func mappingFieldValue(node *yaml.Node, field string) (string, bool) {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == field {
			return node.Content[i+1].Value, true
		}
	}
	return "", false
}

func consistencyEqualsFindings(charter Charter, check CharterCheck, file charterFile, doc *yaml.Node, sourceValues []yamlSelectedValue) ([]Finding, error) {
	expected, ok := stringMapValue(check.Target, "value")
	var targetValues []yamlSelectedValue
	if !ok {
		path := targetYAMLPath(check)
		if path == "" {
			return nil, fmt.Errorf("charter %q check %q: equals requires target.value or target.yaml_path", charter.ID, check.ID)
		}
		var err error
		targetValues, err = yamlPathValues(doc, path)
		if err != nil {
			return nil, fmt.Errorf("charter %q check %q: %w", charter.ID, check.ID, err)
		}
		if len(targetValues) > 0 {
			expected = targetValues[0].value
		}
	}
	var findings []Finding
	for _, source := range sourceValues {
		if source.value == expected {
			continue
		}
		findings = append(findings, consistencyFinding(charter, check, file.display, source.line,
			fmt.Sprintf("value %q does not equal %q", source.value, expected)))
	}
	return findings, nil
}

func consistencyPathFindings(targetDir, root string, charter Charter, check CharterCheck, file charterFile, sourceValues []yamlSelectedValue) []Finding {
	var findings []Finding
	for _, source := range sourceValues {
		path := consistencyPathValue(check, source.value)
		if _, err := os.Stat(resolveConsistencyPath(targetDir, root, check, path)); err == nil {
			continue
		}
		findings = append(findings, consistencyFinding(charter, check, file.display, source.line,
			fmt.Sprintf("required path %q does not exist", path)))
	}
	return findings
}

func consistencyRequiredWhenFindings(targetDir, root string, charter Charter, check CharterCheck, file charterFile, doc *yaml.Node, sourceValues []yamlSelectedValue) ([]Finding, error) {
	for _, source := range sourceValues {
		if !truthyYAMLValue(source.value) {
			return nil, nil
		}
	}
	if path, ok := stringMapValue(check.Target, "yaml_path"); ok && path != "" {
		values, err := yamlPathValues(doc, path)
		if err != nil {
			return nil, fmt.Errorf("charter %q check %q: %w", charter.ID, check.ID, err)
		}
		if len(values) > 0 && truthyYAMLValue(values[0].value) {
			return nil, nil
		}
		return []Finding{consistencyFinding(charter, check, file.display, firstLine(sourceValues), fmt.Sprintf("required target %q is missing", path))}, nil
	}
	if path, ok := stringMapValue(check.Target, "path"); ok && path != "" {
		if _, err := os.Stat(resolveConsistencyPath(targetDir, root, check, path)); err == nil {
			return nil, nil
		}
		return []Finding{consistencyFinding(charter, check, file.display, firstLine(sourceValues), fmt.Sprintf("required path %q does not exist", path))}, nil
	}
	return nil, fmt.Errorf("charter %q check %q: required_when requires target.yaml_path or target.path", charter.ID, check.ID)
}

func consistencyPathValue(check CharterCheck, value string) string {
	template, ok := stringMapValue(check.Target, "path_template")
	if !ok || template == "" {
		return value
	}
	return strings.ReplaceAll(template, "{}", value)
}

func resolveConsistencyPath(targetDir, root string, check CharterCheck, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	if base, ok := stringMapValue(check.Target, "root"); ok && base != "" {
		return resolveCharterPath(targetDir, root, filepath.Join(base, path))
	}
	return resolveCharterPath(targetDir, root, path)
}

func truthyYAMLValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "false", "0", "no", "null":
		return false
	default:
		return true
	}
}

func firstLine(values []yamlSelectedValue) int {
	if len(values) == 0 {
		return 0
	}
	return values[0].line
}

func consistencyFinding(charter Charter, check CharterCheck, file string, line int, fallback string) Finding {
	message := check.Message
	if message == "" {
		message = fallback
	}
	return Finding{
		Check:   check.ID,
		Level:   check.Severity,
		Message: message,
		SuiteID: charter.ID,
		CheckID: check.ID,
		Kind:    check.Kind,
		File:    file,
		Line:    line,
	}
}
