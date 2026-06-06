// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"regexp"
	"strings"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

var (
	reTestPass = regexp.MustCompile(`(?m)^--- PASS:`)
	reTestFail = regexp.MustCompile(`(?m)^--- FAIL:`)
	reTestSkip = regexp.MustCompile(`(?m)^--- SKIP:`)

	// Go build errors look like: file.go:10:5: error message
	reBuildError = regexp.MustCompile(`(?m)^([^\s:]+\.go):\d+:\d+:`)

	// "FAIL" at package level with no individual test results means
	// compilation failed (e.g. "FAIL\tbuild failed")
	reBuildFailed = regexp.MustCompile(`(?m)^FAIL\s+.*\[build failed\]`)
)

// ParseTestMetrics extracts pass/fail/skip counts from go test output.
// If the output indicates a build failure (no tests ran), Total is 0
// and Details["build_failed"] is true.
func ParseTestMetrics(output string) *core.ToolMetrics {
	if reBuildFailed.MatchString(output) {
		return &core.ToolMetrics{
			Total:  0,
			Passed: 0,
			Failed: 0,
			Details: map[string]any{
				"build_failed": true,
			},
		}
	}

	passed := len(reTestPass.FindAllString(output, -1))
	failed := len(reTestFail.FindAllString(output, -1))
	skipped := len(reTestSkip.FindAllString(output, -1))
	total := passed + failed + skipped

	m := &core.ToolMetrics{
		Total:  total,
		Passed: passed,
		Failed: failed,
	}
	if skipped > 0 {
		m.Details = map[string]any{"skipped": skipped}
	}
	return m
}

// ParseBuildMetrics extracts file-level error counts from go build output.
// Total is the number of unique files mentioned in errors.
// Failed equals Total (every mentioned file has at least one error).
// Passed is 0 (go build doesn't list successful files).
func ParseBuildMetrics(output string) *core.ToolMetrics {
	if strings.TrimSpace(output) == "" {
		return &core.ToolMetrics{Total: 0, Passed: 0, Failed: 0}
	}

	matches := reBuildError.FindAllStringSubmatch(output, -1)
	files := make(map[string]struct{})
	for _, m := range matches {
		files[m[1]] = struct{}{}
	}
	errorLines := len(strings.Split(strings.TrimSpace(output), "\n"))

	return &core.ToolMetrics{
		Total:  len(files),
		Passed: 0,
		Failed: len(files),
		Details: map[string]any{
			"error_lines": errorLines,
		},
	}
}
