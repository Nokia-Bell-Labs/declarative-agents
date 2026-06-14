// Copyright (c) 2026 Nokia. All rights reserved.

package exec

import (
	"regexp"
	"strings"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

var (
	reTestPass = regexp.MustCompile(`(?m)^--- PASS:`)
	reTestFail = regexp.MustCompile(`(?m)^--- FAIL:`)
	reTestSkip = regexp.MustCompile(`(?m)^--- SKIP:`)

	reBuildError  = regexp.MustCompile(`(?m)^([^\s:]+\.go):\d+:\d+:`)
	reBuildFailed = regexp.MustCompile(`(?m)^FAIL\s+.*\[build failed\]`)
)

// ParseTestMetrics extracts pass/fail/skip counts from go test output.
func ParseTestMetrics(output string) *core.ToolMetrics {
	if reBuildFailed.MatchString(output) {
		return &core.ToolMetrics{
			Details: map[string]any{"build_failed": true},
		}
	}
	passed := len(reTestPass.FindAllString(output, -1))
	failed := len(reTestFail.FindAllString(output, -1))
	skipped := len(reTestSkip.FindAllString(output, -1))
	m := &core.ToolMetrics{Total: passed + failed + skipped, Passed: passed, Failed: failed}
	if skipped > 0 {
		m.Details = map[string]any{"skipped": skipped}
	}
	return m
}

// ParseBuildMetrics extracts file-level error counts from go build output.
func ParseBuildMetrics(output string) *core.ToolMetrics {
	if strings.TrimSpace(output) == "" {
		return &core.ToolMetrics{}
	}
	files := buildErrorFiles(output)
	return &core.ToolMetrics{
		Total:  len(files),
		Passed: 0,
		Failed: len(files),
		Details: map[string]any{
			"error_lines": len(strings.Split(strings.TrimSpace(output), "\n")),
		},
	}
}

func buildErrorFiles(output string) map[string]struct{} {
	matches := reBuildError.FindAllStringSubmatch(output, -1)
	files := make(map[string]struct{})
	for _, m := range matches {
		files[m[1]] = struct{}{}
	}
	return files
}
