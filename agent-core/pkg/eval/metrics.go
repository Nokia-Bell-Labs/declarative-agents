// Copyright (c) 2026 Nokia. All rights reserved.

package eval

import (
	"regexp"
	"strings"
)

// ToolSnapshot captures the state of a tool at a single invocation point.
type ToolSnapshot struct {
	Tool   string `json:"tool"`
	Signal string `json:"signal"`
	Total  int    `json:"total"`
	Passed int    `json:"passed"`
	Failed int    `json:"failed"`
}

// ExtractToolSnapshots returns a chronological sequence of tool invocation
// snapshots from trace spans. Prefers structured tool.metrics.* attributes
// and falls back to text parsing for legacy traces.
func ExtractToolSnapshots(spans []*Span) []ToolSnapshot {
	tools := ToolSpans(spans)
	var snapshots []ToolSnapshot

	for _, s := range tools {
		name := StrAttr(s, "command.name")
		signal := StrAttr(s, "command.signal")

		if !isMetricTool(name) {
			continue
		}

		snap := ToolSnapshot{Tool: name, Signal: signal}

		if HasAttr(s, "tool.metrics.total") {
			snap.Total = IntAttr(s, "tool.metrics.total")
			snap.Passed = IntAttr(s, "tool.metrics.passed")
			snap.Failed = IntAttr(s, "tool.metrics.failed")
		} else {
			output := StrAttr(s, "tool.output")
			if output == "" {
				output = findOutputFromSibling(s, spans)
			}
			if output != "" {
				snap = parseLegacyOutput(name, signal, output)
			}
		}

		snapshots = append(snapshots, snap)
	}

	return snapshots
}

func isMetricTool(name string) bool {
	switch name {
	case "test", "build", "edit":
		return true
	}
	return false
}

func findOutputFromSibling(target *Span, spans []*Span) string {
	if target.Parent.SpanID == "" {
		return ""
	}
	for _, s := range spans {
		if s == target {
			continue
		}
		if s.Parent.SpanID == target.Parent.SpanID &&
			strings.HasPrefix(s.Name, "dispatch/") {
			out := StrAttr(s, "tool.output")
			if out != "" {
				return out
			}
		}
	}
	return ""
}

var (
	reTestPass   = regexp.MustCompile(`(?m)^--- PASS:`)
	reTestFail   = regexp.MustCompile(`(?m)^--- FAIL:`)
	reBuildFail  = regexp.MustCompile(`(?m)FAIL\s+.*\[build failed\]`)
	reBuildError = regexp.MustCompile(`(?m)^([^\s:]+\.go):\d+:\d+:`)
)

func parseLegacyOutput(tool, signal, output string) ToolSnapshot {
	snap := ToolSnapshot{Tool: tool, Signal: signal}

	switch tool {
	case "test":
		if reBuildFail.MatchString(output) {
			return snap
		}
		passed := len(reTestPass.FindAllString(output, -1))
		failed := len(reTestFail.FindAllString(output, -1))
		snap.Total = passed + failed
		snap.Passed = passed
		snap.Failed = failed

	case "build":
		matches := reBuildError.FindAllStringSubmatch(output, -1)
		files := make(map[string]struct{})
		for _, m := range matches {
			files[m[1]] = struct{}{}
		}
		snap.Total = len(files)
		snap.Failed = len(files)

	case "edit":
		snap.Total = 1
		if signal == "EditDone" || signal == "ToolDone" {
			snap.Passed = 1
		} else {
			snap.Failed = 1
		}
	}

	return snap
}
