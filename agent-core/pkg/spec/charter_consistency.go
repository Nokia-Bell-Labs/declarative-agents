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
}

func parseYAMLPath(path string) ([]yamlPathSegment, error) {
	if !strings.HasPrefix(path, "$.") {
		return nil, fmt.Errorf("yaml_path %q must start with $.", path)
	}
	rawSegments := strings.Split(strings.TrimPrefix(path, "$."), ".")
	segments := make([]yamlPathSegment, 0, len(rawSegments))
	for _, raw := range rawSegments {
		if raw == "" {
			return nil, fmt.Errorf("yaml_path %q contains an empty segment", path)
		}
		segment := yamlPathSegment{key: raw}
		if strings.HasSuffix(raw, "[*]") {
			segment.key = strings.TrimSuffix(raw, "[*]")
			segment.wildcard = true
		}
		segments = append(segments, segment)
	}
	return segments, nil
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
