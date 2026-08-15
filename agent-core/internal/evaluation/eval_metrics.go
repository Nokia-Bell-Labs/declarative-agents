// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package evaluation

// ToolSnapshot captures the state of a tool at a single invocation point.
type ToolSnapshot struct {
	Tool   string `json:"tool"`
	Signal string `json:"signal"`
	Total  int    `json:"total"`
	Passed int    `json:"passed"`
	Failed int    `json:"failed"`
}

// ExtractToolSnapshots returns a chronological sequence of tool invocation
// snapshots from trace spans using provider-neutral structured metrics.
func ExtractToolSnapshots(spans []*Span) []ToolSnapshot {
	tools := ToolSpans(spans)
	var snapshots []ToolSnapshot

	for _, s := range tools {
		name := StrAttr(s, "command.name")
		signal := StrAttr(s, "command.signal")

		if !HasAttr(s, "tool.metrics.total") {
			continue
		}

		snap := ToolSnapshot{
			Tool: name, Signal: signal,
			Total:  IntAttr(s, "tool.metrics.total"),
			Passed: IntAttr(s, "tool.metrics.passed"),
			Failed: IntAttr(s, "tool.metrics.failed"),
		}

		snapshots = append(snapshots, snap)
	}

	return snapshots
}
