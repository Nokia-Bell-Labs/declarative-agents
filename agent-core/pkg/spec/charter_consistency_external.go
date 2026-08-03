// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConsistencyScanPlan lowers one consistency_check into an external file scan
// and focused YAML/provenance reduction.
type ConsistencyScanPlan struct {
	SuiteID     string       `json:"suite_id"`
	Check       CharterCheck `json:"check"`
	Path        string       `json:"path"`
	DisplayRoot string       `json:"display_root"`
	IncludeGlob string       `json:"include_glob,omitempty"`
	ExcludeGlob string       `json:"exclude_glob,omitempty"`
	SourceGlob  string       `json:"source_glob,omitempty"`
}

// BuildConsistencyScanPlans validates source paths without opening target files.
func BuildConsistencyScanPlans(targetDir string, charters []Charter) ([]ConsistencyScanPlan, error) {
	plans := make([]ConsistencyScanPlan, 0)
	for _, charter := range charters {
		for _, check := range charter.Checks {
			if check.Kind != "consistency_check" {
				continue
			}
			if _, err := parseYAMLPath(sourceYAMLPath(check)); err != nil {
				return nil, fmt.Errorf("charter %q check %q: %w", charter.ID, check.ID, err)
			}
			path, displayRoot, err := grepSearchRoot(targetDir, charter, check)
			if err != nil {
				return nil, err
			}
			include, exclude := effectiveGrepGlobs(charter, check, path)
			sourceGlob, err := consistencySourceGlob(targetDir, path, check)
			if err != nil {
				return nil, fmt.Errorf("charter %q check %q: %w", charter.ID, check.ID, err)
			}
			plans = append(plans, ConsistencyScanPlan{
				SuiteID: charter.ID, Check: check, Path: path, DisplayRoot: displayRoot,
				IncludeGlob: combineGrepGlobs(include, false),
				ExcludeGlob: combineGrepGlobs(exclude, true),
				SourceGlob:  sourceGlob,
			})
		}
	}
	return plans, nil
}

func consistencySourceGlob(targetDir, path string, check CharterCheck) (string, error) {
	source, ok := stringMapValue(check.Source, "file")
	if !ok || source == "" {
		return "", nil
	}
	source = filepath.Clean(source)
	if filepath.IsAbs(source) {
		root := path
		if !filepath.IsAbs(root) {
			root = filepath.Join(targetDir, root)
		}
		rel, err := filepath.Rel(root, source)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("source file %q is outside target root", source)
		}
		source = rel
	}
	globs := []string{filepath.ToSlash(source)}
	if !filepath.IsAbs(path) && path != "." {
		globs = prefixGrepGlobs(path, globs)
	}
	return combineGrepGlobs(globs, false), nil
}

type scannedConsistencyFile struct {
	rel     string
	display string
	data    []byte
}

type consistencyScanEvidence struct {
	files     []scannedConsistencyFile
	inventory map[string]bool
}

// ReduceConsistencyScan evaluates YAML rules over externally loaded file
// evidence. Filesystem existence is checked against that same declared scan.
func ReduceConsistencyScan(plan ConsistencyScanPlan, output string) ([]Finding, error) {
	evidence, err := parseConsistencyScan(plan, output)
	if err != nil {
		return nil, fmt.Errorf("charter %q check %q: %w", plan.SuiteID, plan.Check.ID, err)
	}
	if source, ok := stringMapValue(plan.Check.Source, "file"); ok && source != "" && len(evidence.files) == 0 {
		return nil, fmt.Errorf("charter %q check %q: source file %q was not present in external scan",
			plan.SuiteID, plan.Check.ID, source)
	}
	var findings []Finding
	for _, file := range evidence.files {
		fileFindings, err := reduceConsistencyFile(plan, file, evidence.inventory)
		if err != nil {
			return nil, err
		}
		findings = append(findings, fileFindings...)
	}
	SortFindings(findings)
	return findings, nil
}

func parseConsistencyScan(plan ConsistencyScanPlan, output string) (consistencyScanEvidence, error) {
	evidence := consistencyScanEvidence{inventory: map[string]bool{}}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 2 {
			return evidence, fmt.Errorf("invalid consistency scan record")
		}
		pathData, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return evidence, fmt.Errorf("decode scanned path: %w", err)
		}
		rel := filepath.ToSlash(filepath.Clean(string(pathData)))
		switch parts[0] {
		case "I":
			evidence.inventory[rel] = true
		case "F":
			if len(parts) != 3 {
				return evidence, fmt.Errorf("invalid consistency file record for %q", rel)
			}
			data, err := base64.StdEncoding.DecodeString(parts[2])
			if err != nil {
				return evidence, fmt.Errorf("decode scanned file %q: %w", rel, err)
			}
			evidence.files = append(evidence.files, scannedConsistencyFile{
				rel: rel, display: displayCharterPath(plan.DisplayRoot, rel), data: data,
			})
		default:
			return evidence, fmt.Errorf("unknown consistency scan record %q", parts[0])
		}
	}
	sort.Slice(evidence.files, func(i, j int) bool {
		return evidence.files[i].display < evidence.files[j].display
	})
	return evidence, nil
}

func reduceConsistencyFile(
	plan ConsistencyScanPlan,
	file scannedConsistencyFile,
	inventory map[string]bool,
) ([]Finding, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(file.data, &document); err != nil {
		return nil, fmt.Errorf("charter %q check %q: parse YAML file %s: %w",
			plan.SuiteID, plan.Check.ID, file.display, err)
	}
	doc := &document
	if len(document.Content) > 0 {
		doc = document.Content[0]
	}
	values, err := yamlPathValues(doc, sourceYAMLPath(plan.Check))
	if err != nil {
		return nil, fmt.Errorf("charter %q check %q: %w", plan.SuiteID, plan.Check.ID, err)
	}
	charter := Charter{ID: plan.SuiteID}
	evidence := charterFile{rel: file.rel, display: file.display}
	switch plan.Check.Rule {
	case "equals":
		return consistencyEqualsFindings(charter, plan.Check, evidence, doc, values)
	case "required_path_exists":
		return consistencyInventoryPathFindings(charter, plan.Check, evidence, values, inventory), nil
	case "required_when":
		return consistencyInventoryRequiredWhen(charter, plan.Check, evidence, doc, values, inventory)
	default:
		return nil, fmt.Errorf("charter %q check %q: unknown consistency_check rule %q",
			plan.SuiteID, plan.Check.ID, plan.Check.Rule)
	}
}

func consistencyInventoryPathFindings(
	charter Charter,
	check CharterCheck,
	file charterFile,
	values []yamlSelectedValue,
	inventory map[string]bool,
) []Finding {
	var findings []Finding
	for _, source := range values {
		path := consistencyPathValue(check, source.value)
		if inventory[consistencyInventoryPath(check, path)] {
			continue
		}
		findings = append(findings, consistencyFinding(charter, check, file.display, source.line,
			fmt.Sprintf("required path %q does not exist", path)))
	}
	return findings
}

func consistencyInventoryRequiredWhen(
	charter Charter,
	check CharterCheck,
	file charterFile,
	doc *yaml.Node,
	values []yamlSelectedValue,
	inventory map[string]bool,
) ([]Finding, error) {
	for _, source := range values {
		if !truthyYAMLValue(source.value) {
			return nil, nil
		}
	}
	if path, ok := stringMapValue(check.Target, "yaml_path"); ok && path != "" {
		target, err := yamlPathValues(doc, path)
		if err != nil {
			return nil, err
		}
		if len(target) > 0 && truthyYAMLValue(target[0].value) {
			return nil, nil
		}
		return []Finding{consistencyFinding(charter, check, file.display, firstLine(values),
			fmt.Sprintf("required target %q is missing", path))}, nil
	}
	if path, ok := stringMapValue(check.Target, "path"); ok && path != "" {
		if inventory[consistencyInventoryPath(check, path)] {
			return nil, nil
		}
		return []Finding{consistencyFinding(charter, check, file.display, firstLine(values),
			fmt.Sprintf("required path %q does not exist", path))}, nil
	}
	return nil, fmt.Errorf("charter %q check %q: required_when requires target.yaml_path or target.path",
		charter.ID, check.ID)
}

func consistencyInventoryPath(check CharterCheck, path string) string {
	if base, ok := stringMapValue(check.Target, "root"); ok && base != "" {
		path = filepath.Join(base, path)
	}
	return filepath.ToSlash(filepath.Clean(path))
}
