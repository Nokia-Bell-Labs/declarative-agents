// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package spec

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	refTargetMarker = "@@TARGET@@"
	refFileMarker   = "@@REFERENCE_FILE@@"
	refDirMarker    = "@@REFERENCE_DIRECTORY@@"
)

// RefSearchPlan lowers one ref_check into a declared filesystem scan plus a
// focused finding reducer.
type RefSearchPlan struct {
	SuiteID         string   `json:"suite_id"`
	CheckID         string   `json:"check_id"`
	Kind            string   `json:"kind"`
	Severity        string   `json:"severity"`
	Message         string   `json:"message,omitempty"`
	Path            string   `json:"path"`
	DisplayRoot     string   `json:"display_root"`
	IncludeGlob     string   `json:"include_glob"`
	ExcludeGlob     string   `json:"exclude_glob"`
	Query           string   `json:"query"`
	Group           int      `json:"group"`
	AllowMissing    bool     `json:"allow_missing"`
	Allowed         []string `json:"allowed"`
	ReferenceFile   string   `json:"reference_file"`
	ReferenceDir    string   `json:"reference_dir"`
	ReferenceFormat string   `json:"reference_format"`
}

// BuildRefSearchPlans validates and lowers ref_check policy without scanning
// target or inventory files.
func BuildRefSearchPlans(targetDir string, charters []Charter) ([]RefSearchPlan, error) {
	plans := make([]RefSearchPlan, 0)
	for _, charter := range charters {
		for _, check := range charter.Checks {
			if check.Kind != "ref_check" {
				continue
			}
			plan, err := buildRefSearchPlan(targetDir, charter, check)
			if err != nil {
				return nil, err
			}
			plans = append(plans, plan)
		}
	}
	return plans, nil
}

func buildRefSearchPlan(targetDir string, charter Charter, check CharterCheck) (RefSearchPlan, error) {
	extractor, err := refExtractor(check)
	if err != nil {
		return RefSearchPlan{}, fmt.Errorf("charter %q check %q: %w", charter.ID, check.ID, err)
	}
	if len(check.Refs) == 0 {
		return RefSearchPlan{}, fmt.Errorf("charter %q check %q: ref_check requires references", charter.ID, check.ID)
	}
	path, displayRoot, err := grepSearchRoot(targetDir, charter, check)
	if err != nil {
		return RefSearchPlan{}, err
	}
	include, exclude := effectiveGrepGlobs(charter, check, path)
	allowed := make([]string, 0)
	for _, key := range []string{"values", "keys", "inline"} {
		allowed = append(allowed, stringSliceMapValue(check.Refs, key)...)
	}
	sort.Strings(allowed)
	refFile, _ := stringMapValue(check.Refs, "file")
	refDir, _ := stringMapValue(check.Refs, "directory")
	format, _ := stringMapValue(check.Refs, "format")
	inventoryRoot := path
	if !filepath.IsAbs(inventoryRoot) {
		inventoryRoot = filepath.Join(targetDir, inventoryRoot)
	}
	return RefSearchPlan{
		SuiteID: charter.ID, CheckID: check.ID, Kind: check.Kind,
		Severity: check.Severity, Message: check.Message,
		Path: path, DisplayRoot: displayRoot,
		IncludeGlob: combineGrepGlobs(include, false),
		ExcludeGlob: combineGrepGlobs(exclude, true),
		Query:       extractor.re.String(), Group: extractor.group,
		AllowMissing: check.AllowMissing, Allowed: allowed,
		ReferenceFile:   resolvedOptionalCharterPath(targetDir, inventoryRoot, refFile),
		ReferenceDir:    resolvedOptionalCharterPath(targetDir, inventoryRoot, refDir),
		ReferenceFormat: format,
	}, nil
}

func resolvedOptionalCharterPath(targetDir, root, path string) string {
	if path == "" {
		return ""
	}
	return resolveCharterPath(targetDir, root, path)
}

// ReduceRefSearch combines declared rg evidence and externally loaded
// inventories into deterministic ref_check findings.
func ReduceRefSearch(plan RefSearchPlan, output string) ([]Finding, error) {
	target, fileInventory, dirInventory, err := splitRefSearchOutput(output)
	if err != nil {
		return nil, fmt.Errorf("charter %q check %q: %w", plan.SuiteID, plan.CheckID, err)
	}
	allowed := refAllowedValues(plan, fileInventory, dirInventory)
	if len(allowed) == 0 {
		return nil, fmt.Errorf("charter %q check %q: ref_check references produced no allowed values", plan.SuiteID, plan.CheckID)
	}
	check := CharterCheck{
		ID: plan.CheckID, Kind: plan.Kind, Severity: plan.Severity,
		Message: plan.Message, Extract: map[string]any{"regex": plan.Query, "group": plan.Group},
	}
	extractor, err := refExtractor(check)
	if err != nil {
		return nil, err
	}
	events, err := parseGrepEvents(GrepSearchPlan{
		SuiteID: plan.SuiteID, CheckID: plan.CheckID, DisplayRoot: plan.DisplayRoot,
	}, target)
	if err != nil {
		return nil, err
	}
	return reduceRefEvents(plan, check, extractor, allowed, events), nil
}

func refAllowedValues(plan RefSearchPlan, fileInventory, dirInventory string) map[string]bool {
	allowed := make(map[string]bool, len(plan.Allowed))
	for _, value := range plan.Allowed {
		allowed[value] = true
	}
	fileValues := inventoryFileValues(fileInventory, plan.ReferenceFormat)
	for _, value := range append(fileValues, inventoryLines(dirInventory)...) {
		allowed[value] = true
	}
	return allowed
}

func reduceRefEvents(
	plan RefSearchPlan,
	check CharterCheck,
	extractor refRegexExtractor,
	allowed map[string]bool,
	events []grepMatchEvent,
) []Finding {
	charter := Charter{ID: plan.SuiteID}
	var findings []Finding
	extracted := 0
	for _, event := range events {
		for _, ref := range extractor.extract(event.Line) {
			extracted++
			if !allowed[ref] {
				findings = append(findings, refFinding(charter, check, event.Path, event.LineNumber, ref))
			}
		}
	}
	if extracted == 0 && !plan.AllowMissing {
		findings = append(findings, refFinding(charter, check, "", 0, ""))
	}
	return findings
}

func splitRefSearchOutput(output string) (string, string, string, error) {
	if !strings.HasPrefix(output, refTargetMarker) {
		return "", "", "", fmt.Errorf("ref scan output is missing target marker")
	}
	fileAt := strings.Index(output, refFileMarker)
	dirAt := strings.Index(output, refDirMarker)
	if fileAt < 0 || dirAt < fileAt {
		return "", "", "", fmt.Errorf("ref scan output is missing inventory markers")
	}
	return strings.TrimPrefix(output[len(refTargetMarker):fileAt], "\n"),
		strings.TrimPrefix(output[fileAt+len(refFileMarker):dirAt], "\n"),
		strings.TrimPrefix(output[dirAt+len(refDirMarker):], "\n"), nil
}

func inventoryFileValues(data, format string) []string {
	if format == "bibtex_keys" {
		return bibtexKeys(data)
	}
	return inventoryLines(data)
}

func inventoryLines(data string) []string {
	var values []string
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			values = append(values, line)
		}
	}
	sort.Strings(values)
	return values
}

type refRegexExtractor struct {
	re    *regexp.Regexp
	group int
}

func refExtractor(check CharterCheck) (refRegexExtractor, error) {
	raw, ok := stringMapValue(check.Extract, "regex")
	if !ok || raw == "" {
		return refRegexExtractor{}, fmt.Errorf("ref_check requires extract.regex")
	}
	group := 1
	if configured, ok := intMapValue(check.Extract, "group"); ok {
		group = configured
	}
	re, err := regexp.Compile(raw)
	if err != nil {
		return refRegexExtractor{}, fmt.Errorf("invalid extract regex %q: %w", raw, err)
	}
	if group < 0 || group > re.NumSubexp() {
		return refRegexExtractor{}, fmt.Errorf("extract group %d out of range for regex %q", group, raw)
	}
	return refRegexExtractor{re: re, group: group}, nil
}

func (e refRegexExtractor) extract(line string) []string {
	matches := e.re.FindAllStringSubmatch(line, -1)
	refs := make([]string, 0, len(matches))
	for _, match := range matches {
		if e.group >= len(match) {
			continue
		}
		ref := strings.TrimSpace(match[e.group])
		if ref != "" {
			refs = append(refs, ref)
		}
	}
	return refs
}

func bibtexKeys(data string) []string {
	re := regexp.MustCompile(`@[A-Za-z]+\s*\{\s*([^,\s]+)`)
	matches := re.FindAllStringSubmatch(data, -1)
	keys := make([]string, 0, len(matches))
	for _, match := range matches {
		keys = append(keys, match[1])
	}
	sort.Strings(keys)
	return keys
}

func resolveCharterPath(targetDir, root, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	base := targetDir
	if root != "" {
		base = root
	}
	return filepath.Join(base, path)
}

func refFinding(charter Charter, check CharterCheck, file string, line int, ref string) Finding {
	message := check.Message
	if message == "" {
		if ref == "" {
			message = "no references found"
		} else {
			message = fmt.Sprintf("reference %q does not resolve", ref)
		}
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

func stringMapValue(values map[string]any, key string) (string, bool) {
	value, ok := values[key]
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	return text, ok
}

func intMapValue(values map[string]any, key string) (int, bool) {
	value, ok := values[key]
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	default:
		return 0, false
	}
}

func stringSliceMapValue(values map[string]any, key string) []string {
	value, ok := values[key]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}
