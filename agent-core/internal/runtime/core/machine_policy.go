// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"fmt"
	"time"
)

func (bs *BudgetSpec) CommandTimeoutDuration() time.Duration {
	if bs == nil || bs.CommandTimeout == "" {
		return 0
	}
	timeout, _ := time.ParseDuration(bs.CommandTimeout)
	return timeout
}

func validateMachinePolicy(spec MachineSpec, signals map[string]bool) []string {
	var errs []string
	for field, signal := range map[string]string{
		"summary_signal": spec.SummarySignal, "resume_signal": spec.ResumeSignal,
	} {
		if signal != "" && !signals[signal] {
			errs = append(errs, fmt.Sprintf("%s %q not in signals list", field, signal))
		}
	}
	if spec.BudgetSpec != nil && spec.BudgetSpec.CommandTimeout != "" {
		timeout, err := time.ParseDuration(spec.BudgetSpec.CommandTimeout)
		if err != nil || timeout <= 0 {
			errs = append(errs, fmt.Sprintf(
				"budget.command_timeout: invalid positive duration %q",
				spec.BudgetSpec.CommandTimeout,
			))
		}
	}
	return errs
}

func validateReportOutput(index int, transition TransitionSpec) string {
	if transition.ReportOutput == "" {
		return ""
	}
	selector, ok := ParseSelector(transition.ReportOutput)
	if !ok || selector.Label != "" {
		return fmt.Sprintf(
			"transition[%d].report_output: %q must be a $.path selector",
			index, transition.ReportOutput,
		)
	}
	if transition.Action == "" {
		return fmt.Sprintf("transition[%d].report_output requires an action", index)
	}
	return ""
}

func machinePolicyDiagnostics(spec MachineSpec) []MachineDiagnostic {
	var diagnostics []MachineDiagnostic
	policies := []struct {
		code    string
		missing bool
	}{
		{DiagnosticImplicitSummarySignal, spec.SummarySignal == ""},
		{DiagnosticImplicitResumeSignal, spec.ResumeSignal == ""},
		{DiagnosticImplicitCommandTimeout, spec.BudgetSpec == nil || spec.BudgetSpec.CommandTimeout == ""},
		{DiagnosticImplicitMaxIterations, spec.BudgetSpec == nil || spec.BudgetSpec.MaxIterations == 0},
	}
	for _, policy := range policies {
		if policy.missing {
			diagnostics = append(diagnostics, MachineDiagnostic{
				Severity: MachineDiagnosticWarning, Code: policy.code,
				Message: "machine uses the documented runtime default",
			})
		}
	}
	for _, terminal := range spec.TerminalStates {
		if _, declared := DeclaredTerminalStatus(&spec, State(terminal)); !declared {
			diagnostics = append(diagnostics, MachineDiagnostic{
				Severity: MachineDiagnosticWarning, Code: DiagnosticMissingTerminalStatus,
				Message: fmt.Sprintf("terminal state %q uses legacy name-based run status", terminal),
				State:   terminal,
			})
		}
	}
	return diagnostics
}
