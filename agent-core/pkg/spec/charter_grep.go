// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
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

// ExecuteGrepChecks runs grep_check charter checks over targetDir.
func ExecuteGrepChecks(targetDir string, charters []Charter) ([]Finding, error) {
	plans, err := BuildGrepSearchPlans(targetDir, charters)
	if err != nil {
		return nil, err
	}
	var findings []Finding
	for _, plan := range plans {
		cmd := exec.Command("rg", plan.args()...)
		cmd.Dir = targetDir
		output, runErr := cmd.CombinedOutput()
		exitCode := 0
		if runErr != nil {
			var exitErr *exec.ExitError
			if !errors.As(runErr, &exitErr) {
				return nil, fmt.Errorf("charter %q check %q: run rg: %w", plan.SuiteID, plan.CheckID, runErr)
			}
			exitCode = exitErr.ExitCode()
		}
		checkFindings, err := ReduceGrepSearch(plan, string(output), exitCode)
		if err != nil {
			return nil, err
		}
		findings = append(findings, checkFindings...)
	}
	sort.Slice(findings, func(i, j int) bool {
		return findingLess(findings[i], findings[j])
	})
	return findings, nil
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
			plan, err := buildGrepSearchPlan(targetDir, charter, check)
			if err != nil {
				return nil, err
			}
			plans = append(plans, plan)
		}
	}
	return plans, nil
}

func buildGrepSearchPlan(targetDir string, charter Charter, check CharterCheck) (GrepSearchPlan, error) {
	mode, err := validateGrepCheck(charter, check)
	if err != nil {
		return GrepSearchPlan{}, err
	}
	path, displayRoot, err := grepSearchRoot(targetDir, charter, check)
	if err != nil {
		return GrepSearchPlan{}, err
	}
	include, exclude := effectiveGrepGlobs(charter, check, path)
	return GrepSearchPlan{
		SuiteID: charter.ID, CheckID: check.ID, Kind: check.Kind,
		Severity: check.Severity, Message: check.Message, Mode: mode,
		Patterns: append([]string(nil), check.Patterns...), Regex: check.Regex,
		Query: grepQuery(check), Path: path, DisplayRoot: displayRoot,
		IncludeGlob: combineGrepGlobs(include, false),
		ExcludeGlob: combineGrepGlobs(exclude, true),
	}, nil
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
	if _, err := compileGrepPatterns(check); err != nil {
		return "", fmt.Errorf("charter %q check %q: %w", charter.ID, check.ID, err)
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

func grepQuery(check CharterCheck) string {
	queryParts := make([]string, len(check.Patterns))
	for i, pattern := range check.Patterns {
		if !check.Regex {
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

func (plan GrepSearchPlan) args() []string {
	args := []string{"--json", "--line-number", "--no-ignore", "--hidden", "--text", "--threads", "1", "--sort", "path"}
	if plan.IncludeGlob != "" {
		args = append(args, "--glob", plan.IncludeGlob)
	}
	if plan.ExcludeGlob != "" {
		args = append(args, "--glob", plan.ExcludeGlob)
	}
	return append(args, "--", plan.Query, plan.Path)
}

// ReduceGrepSearch converts one rg JSON event stream into charter findings.
// It never opens target files; rg is the sole producer of matched line evidence.
func ReduceGrepSearch(plan GrepSearchPlan, output string, exitCode int) ([]Finding, error) {
	if exitCode != 0 && exitCode != 1 {
		return nil, fmt.Errorf("charter %q check %q: rg failed (exit %d): %s",
			plan.SuiteID, plan.CheckID, exitCode, strings.TrimSpace(output))
	}
	check := CharterCheck{Patterns: plan.Patterns, Regex: plan.Regex}
	patterns, err := compileGrepPatterns(check)
	if err != nil {
		return nil, fmt.Errorf("charter %q check %q: %w", plan.SuiteID, plan.CheckID, err)
	}
	events, err := parseGrepEvents(plan, output)
	if err != nil {
		return nil, err
	}
	var findings []Finding
	for _, event := range events {
		if plan.Mode != "match" {
			continue
		}
		findings = append(findings, grepEventFindings(plan, event, patterns)...)
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

func grepEventFindings(plan GrepSearchPlan, event grepMatchEvent, patterns []grepPattern) []Finding {
	var findings []Finding
	for _, pattern := range patterns {
		if pattern.matches(event.Line) {
			findings = append(findings, grepPlanFinding(
				plan, grepDisplayPath(plan, event.Path), event.LineNumber, pattern.raw,
			))
		}
	}
	return findings
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

type grepPattern struct {
	raw string
	re  *regexp.Regexp
}

func compileGrepPatterns(check CharterCheck) ([]grepPattern, error) {
	patterns := make([]grepPattern, 0, len(check.Patterns))
	for _, raw := range check.Patterns {
		if !check.Regex {
			patterns = append(patterns, grepPattern{raw: raw})
			continue
		}
		re, err := regexp.Compile(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid regex pattern %q: %w", raw, err)
		}
		patterns = append(patterns, grepPattern{raw: raw, re: re})
	}
	return patterns, nil
}

func (p grepPattern) matches(line string) bool {
	if p.re != nil {
		return p.re.MatchString(line)
	}
	return strings.Contains(line, p.raw)
}
