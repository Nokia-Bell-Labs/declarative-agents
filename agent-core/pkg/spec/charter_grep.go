// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// GrepSearchPlan is one declarative grep_check lowered to a ripgrep invocation.
// The jurist machine serializes these plans into command state and dispatches
// one visible rg word for each plan.
type GrepSearchPlan struct {
	SuiteID     string   `json:"suite_id"`
	CheckID     string   `json:"check_id"`
	Kind        string   `json:"kind"`
	Severity    string   `json:"severity"`
	Message     string   `json:"message,omitempty"`
	Mode        string   `json:"mode"`
	Patterns    []string `json:"patterns"`
	Regex       bool     `json:"regex"`
	Query       string   `json:"query"`
	Path        string   `json:"path"`
	DisplayRoot string   `json:"display_root"`
	IncludeGlob string   `json:"include_glob"`
	ExcludeGlob string   `json:"exclude_glob"`
}

// BuildGrepSearchPlans validates grep checks and lowers their target policy to
// ripgrep query, path, and glob inputs without reading target files.
func BuildGrepSearchPlans(targetDir string, charters []Charter) ([]GrepSearchPlan, error) {
	plans := make([]GrepSearchPlan, 0)
	for _, charter := range charters {
		for _, check := range charter.Checks {
			if check.Kind != "grep_check" {
				continue
			}
			checkPlans, err := buildGrepSearchPlansForCheck(targetDir, charter, check)
			if err != nil {
				return nil, err
			}
			plans = append(plans, checkPlans...)
		}
	}
	return plans, nil
}

func buildGrepSearchPlansForCheck(targetDir string, charter Charter, check CharterCheck) ([]GrepSearchPlan, error) {
	mode, err := validateGrepCheck(charter, check)
	if err != nil {
		return nil, err
	}
	path, displayRoot, err := grepSearchRoot(targetDir, charter, check)
	if err != nil {
		return nil, err
	}
	include, exclude := effectiveGrepGlobs(charter, check, path)
	base := GrepSearchPlan{
		SuiteID: charter.ID, CheckID: check.ID, Kind: check.Kind,
		Severity: check.Severity, Message: check.Message, Mode: mode,
		Regex: check.Regex, Path: path, DisplayRoot: displayRoot,
		IncludeGlob: combineGrepGlobs(include, false),
		ExcludeGlob: combineGrepGlobs(exclude, true),
	}
	groups := [][]string{check.Patterns}
	if mode == "match" {
		groups = make([][]string, len(check.Patterns))
		for i, pattern := range check.Patterns {
			groups[i] = []string{pattern}
		}
	}
	plans := make([]GrepSearchPlan, 0, len(groups))
	for _, patterns := range groups {
		plan := base
		plan.Patterns = append([]string(nil), patterns...)
		plan.Query = grepQuery(patterns, check.Regex)
		plans = append(plans, plan)
	}
	return plans, nil
}

func validateGrepCheck(charter Charter, check CharterCheck) (string, error) {
	if len(check.Patterns) == 0 {
		return "", fmt.Errorf("charter %q check %q: grep_check requires patterns", charter.ID, check.ID)
	}
	mode := check.Mode
	if mode == "" {
		mode = "match"
	}
	if mode != "match" && mode != "missing" {
		return "", fmt.Errorf("charter %q check %q: unknown grep_check mode %q", charter.ID, check.ID, check.Mode)
	}
	return mode, nil
}

func grepSearchRoot(targetDir string, charter Charter, check CharterCheck) (string, string, error) {
	root := charter.Target.Root
	if root == "" {
		root = "."
	}
	path := filepath.Clean(root)
	displayRoot := filepath.ToSlash(path)
	if filepath.IsAbs(path) {
		displayRoot = "."
	} else if _, err := filepath.Rel(targetDir, filepath.Join(targetDir, path)); err != nil {
		return "", "", fmt.Errorf("charter %q check %q: resolve target root: %w", charter.ID, check.ID, err)
	}
	return path, displayRoot, nil
}

func effectiveGrepGlobs(charter Charter, check CharterCheck, path string) ([]string, []string) {
	include, exclude := charter.Target.Include, charter.Target.Exclude
	if len(check.Include) > 0 || len(check.Exclude) > 0 {
		include, exclude = check.Include, check.Exclude
	}
	if !filepath.IsAbs(path) && path != "." {
		include = prefixGrepGlobs(path, include)
		exclude = prefixGrepGlobs(path, exclude)
	}
	return include, exclude
}

func grepQuery(patterns []string, regex bool) string {
	queryParts := make([]string, len(patterns))
	for i, pattern := range patterns {
		if !regex {
			pattern = regexp.QuoteMeta(pattern)
		}
		queryParts[i] = "(?:" + pattern + ")"
	}
	return strings.Join(queryParts, "|")
}

func prefixGrepGlobs(root string, globs []string) []string {
	prefixed := make([]string, 0, len(globs))
	for _, glob := range globs {
		prefixed = append(prefixed, filepath.ToSlash(filepath.Join(root, glob)))
	}
	return prefixed
}

func combineGrepGlobs(globs []string, exclude bool) string {
	cleaned := make([]string, 0, len(globs))
	for _, glob := range globs {
		if glob = strings.TrimSpace(filepath.ToSlash(glob)); glob != "" {
			cleaned = append(cleaned, glob)
		}
	}
	if len(cleaned) == 0 {
		return ""
	}
	combined := cleaned[0]
	if len(cleaned) > 1 {
		combined = "{" + strings.Join(cleaned, ",") + "}"
	}
	if exclude {
		return "!" + combined
	}
	return combined
}

// ReduceGrepSearch converts one rg JSON event stream into charter findings.
// It never opens target files; rg is the sole producer of matched line evidence.
func ReduceGrepSearch(plan GrepSearchPlan, output string, exitCode int) ([]Finding, error) {
	if exitCode != 0 && exitCode != 1 {
		return nil, fmt.Errorf("charter %q check %q: rg failed (exit %d): %s",
			plan.SuiteID, plan.CheckID, exitCode, strings.TrimSpace(output))
	}
	events, err := parseGrepEvents(plan, output)
	if err != nil {
		return nil, err
	}
	var findings []Finding
	if plan.Mode == "match" && len(plan.Patterns) != 1 {
		return nil, fmt.Errorf("charter %q check %q: match plan requires exactly one attributed pattern",
			plan.SuiteID, plan.CheckID)
	}
	for _, event := range events {
		if plan.Mode != "match" {
			continue
		}
		findings = append(findings, grepPlanFinding(
			plan, grepDisplayPath(plan, event.Path), event.LineNumber, plan.Patterns[0],
		))
	}
	if plan.Mode == "missing" && len(events) == 0 {
		findings = append(findings, grepPlanFinding(plan, "", 0, strings.Join(plan.Patterns, ", ")))
	}
	return findings, nil
}

type grepMatchEvent struct {
	Path       string
	Line       string
	LineNumber int
}

type rgJSONEvent struct {
	Type string `json:"type"`
	Data struct {
		Path struct {
			Text string `json:"text"`
		} `json:"path"`
		Lines struct {
			Text string `json:"text"`
		} `json:"lines"`
		LineNumber int `json:"line_number"`
	} `json:"data"`
}

func parseGrepEvents(plan GrepSearchPlan, output string) ([]grepMatchEvent, error) {
	var events []grepMatchEvent
	scanner := bufio.NewScanner(bytes.NewBufferString(output))
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		var event rgJSONEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, fmt.Errorf("charter %q check %q: decode rg output: %w", plan.SuiteID, plan.CheckID, err)
		}
		if event.Type != "match" {
			continue
		}
		events = append(events, grepMatchEvent{
			Path: event.Data.Path.Text, Line: event.Data.Lines.Text, LineNumber: event.Data.LineNumber,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("charter %q check %q: scan rg output: %w", plan.SuiteID, plan.CheckID, err)
	}
	return events, nil
}

func grepDisplayPath(plan GrepSearchPlan, path string) string {
	path = filepath.Clean(path)
	if filepath.IsAbs(path) {
		if rel, err := filepath.Rel(plan.Path, path); err == nil {
			path = rel
		}
	}
	path = filepath.ToSlash(path)
	if plan.DisplayRoot == "." || plan.DisplayRoot == "" || strings.HasPrefix(path, plan.DisplayRoot+"/") {
		return strings.TrimPrefix(path, "./")
	}
	return filepath.ToSlash(filepath.Join(plan.DisplayRoot, path))
}

func grepPlanFinding(plan GrepSearchPlan, file string, line int, pattern string) Finding {
	message := plan.Message
	if message == "" {
		message = fmt.Sprintf("pattern %q matched", pattern)
	}
	if plan.Mode == "missing" && file == "" {
		message = fmt.Sprintf("pattern %q not found", pattern)
		if plan.Message != "" {
			message = plan.Message
		}
	}
	return Finding{
		Check: plan.CheckID, Level: plan.Severity, Message: message,
		SuiteID: plan.SuiteID, CheckID: plan.CheckID, Kind: plan.Kind,
		File: file, Line: line,
	}
}
