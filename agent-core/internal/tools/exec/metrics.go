// Copyright (c) 2026 Nokia. All rights reserved.

package exec

import (
	"context"
	"errors"
	osexec "os/exec"
	"regexp"
	"strings"
	"time"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/monitor"
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

// SetMonitorRecorder connects exec commands to the embedded monitor recorder.
func (c *ExecCmd) SetMonitorRecorder(rec monitor.ToolMetricsRecorder) {
	c.rec = rec
}

func (c *ExecCmd) recordExecMetrics(duration time.Duration, output []byte, err error) {
	if c.rec == nil {
		return
	}
	values := map[string]float64{
		"process_duration": float64(duration.Milliseconds()),
		"output_bytes":     float64(len(output)),
		"exit_code":        float64(exitCode(err)),
	}
	core.RecordDeclaredToolMetrics(context.Background(), c.rec, c.Name(), c.def.Metrics, values, map[string]string{
		"binary": c.def.Binary,
	})
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *osexec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}
