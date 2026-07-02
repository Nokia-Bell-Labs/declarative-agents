// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// checkToolMetricConfig verifies planned ToolDef metric declarations.
func checkToolMetricConfig(corpus *Corpus) []Finding {
	var findings []Finding
	for name, td := range corpus.ToolDeclarations {
		if err := core.ValidateMetricConfig(name, td.Metrics); err != nil {
			findings = append(findings, Finding{
				Check:   "tool-metric-config-invalid",
				Level:   "error",
				Message: fmt.Sprintf("tool %q metric config invalid: %s", name, err),
			})
		}
	}
	return findings
}

// checkMachineMetricLabels verifies MachineSpec workflow metric labels.
func checkMachineMetricLabels(corpus *Corpus) []Finding {
	var findings []Finding
	for name, ms := range corpus.Machines {
		findings = append(findings, machineMetricLabelFindings(name, ms)...)
	}
	return findings
}

func machineMetricLabelFindings(name string, ms core.MachineSpec) []Finding {
	var findings []Finding
	if err := core.ValidateMetricLabels("metric_labels", ms.MetricLabels); err != nil {
		findings = append(findings, metricLabelFinding(name, err))
	}
	for i, tr := range ms.Transitions {
		owner := fmt.Sprintf("transitions[%d].metric_labels", i)
		if err := core.ValidateMetricLabels(owner, tr.MetricLabels); err != nil {
			findings = append(findings, metricLabelFinding(name, err))
		}
	}
	return findings
}

func metricLabelFinding(machine string, err error) Finding {
	return Finding{
		Check:   "machine-metric-label-invalid",
		Level:   "error",
		Message: fmt.Sprintf("machine %s metric labels invalid: %s", machine, err),
	}
}
