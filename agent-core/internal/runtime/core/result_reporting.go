// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"encoding/json"
	"fmt"
	"strings"
)

func transitionCommandStateLabel(spec *MachineSpec, state State, signal Signal) string {
	if spec == nil {
		return ""
	}
	for _, transition := range spec.Transitions {
		if transition.State == string(state) && transition.Signal == string(signal) {
			return transition.Label
		}
	}
	return ""
}

func transitionReportPolicy(
	spec *MachineSpec, state State, signal Signal,
) (selector, label string) {
	if spec == nil {
		return "", ""
	}
	for _, transition := range spec.Transitions {
		if transition.State == string(state) && transition.Signal == string(signal) {
			return transition.ReportOutput, transition.Label
		}
	}
	return "", ""
}

func transitionSummaryPolicy(spec *MachineSpec, state State, signal Signal) bool {
	if spec == nil {
		return false
	}
	for _, transition := range spec.Transitions {
		if transition.State == string(state) && transition.Signal == string(signal) {
			return transition.Summary
		}
	}
	return false
}

func applyResultPolicies(runner *loopRunner) {
	if runner.summaryOutput {
		runner.run.Summary = runner.result.Output
		runner.summaryOutput = false
	}
	if runner.reportOutput == "" {
		return
	}
	runner.result.OperatorReport = resolveOperatorReport(
		runner.reportOutput, runner.reportLabel, runner.result.Output,
	)
	runner.reportOutput, runner.reportLabel = "", ""
}

func resolveOperatorReport(selector, label, output string) *OperatorReport {
	parsed, ok := ParseSelector(selector)
	if !ok || parsed.Label != "" {
		return nil
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		return nil
	}
	value, ok := parsed.Resolve(decoded)
	if !ok || !operatorReportScalar(value) {
		return nil
	}
	return &OperatorReport{
		Label: label, Field: strings.Join(parsed.Path, "."), Value: fmt.Sprint(value),
	}
}

func operatorReportScalar(value interface{}) bool {
	switch value.(type) {
	case string, bool, float64:
		return true
	default:
		return false
	}
}
