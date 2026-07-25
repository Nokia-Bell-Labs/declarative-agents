// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import (
	"fmt"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

const reportParseErrorTool = "report_parse_error"

// ValidateParseRetryWiring requires a machine with a parse retry budget to make
// retry and exhaustion behavior explicit in its selected tools and grammar.
func ValidateParseRetryWiring(spec core.MachineSpec, defs []ToolDef) error {
	if spec.BudgetSpec == nil || spec.BudgetSpec.MaxConsecutiveParseErrors <= 0 {
		return nil
	}

	reportTransitions := parseErrorReportTransitions(spec.Transitions)
	var problems []string
	if !toolSelected(defs, reportParseErrorTool) {
		problems = append(problems, `select "report_parse_error" in the profile tools`)
	}
	if len(reportTransitions) == 0 {
		problems = append(problems, `route a ParseFailed transition through action "report_parse_error"`)
	}
	problems = append(problems, missingExhaustionRoutes(spec.Transitions, spec.TerminalStates, reportTransitions)...)
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf(
		"parse retry wiring validation: machine %q sets budget.max_consecutive_parse_errors=%d; %s; remove the parse retry budget if the profile does not retry parse failures",
		spec.Name, spec.BudgetSpec.MaxConsecutiveParseErrors, strings.Join(problems, "; "),
	)
}

func toolSelected(defs []ToolDef, name string) bool {
	for _, def := range defs {
		if def.Name == name {
			return true
		}
	}
	return false
}

func parseErrorReportTransitions(transitions []core.TransitionSpec) []core.TransitionSpec {
	var reports []core.TransitionSpec
	for _, tr := range transitions {
		if tr.Signal == string(core.ParseFailed) && tr.Action == reportParseErrorTool {
			reports = append(reports, tr)
		}
	}
	return reports
}

func missingExhaustionRoutes(transitions []core.TransitionSpec, terminalStates []string, reports []core.TransitionSpec) []string {
	var problems []string
	for _, report := range reports {
		if !hasTerminalTransition(transitions, terminalStates, report.Next, string(core.BudgetExhausted)) {
			problems = append(problems, fmt.Sprintf(
				`route BudgetExhausted from state %q (the destination of ParseFailed -> report_parse_error) to a terminal failure state`,
				report.Next,
			))
		}
	}
	return problems
}

func hasTerminalTransition(transitions []core.TransitionSpec, terminalStates []string, state, signal string) bool {
	for _, tr := range transitions {
		if tr.State == state && tr.Signal == signal && containsString(terminalStates, tr.Next) {
			return true
		}
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
