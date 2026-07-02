// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"fmt"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"strings"
)

func checkMachineActionResolution(corpus *Corpus) []Finding {
	var findings []Finding
	for _, agentName := range corpus.MachineOrder {
		ms := corpus.Machines[agentName]
		selected := corpus.ToolSelections[agentName]
		if len(selected) == 0 {
			continue
		}
		toolSet := make(map[string]bool, len(selected))
		for _, t := range selected {
			toolSet[t] = true
		}
		for _, tr := range ms.Transitions {
			if tr.Action == "" {
				continue
			}
			if tr.Action == "$tool" {
				continue
			}
			if !toolSet[tr.Action] {
				findings = append(findings, Finding{
					Check:   "machine-unresolved-action",
					Level:   "error",
					Message: fmt.Sprintf("machine %s transition %s+%s action %q not in tool selection", agentName, tr.State, tr.Signal, tr.Action),
				})
			}
		}
	}
	return findings
}

// checkMachineSignalCoverage verifies that every declared signal is
// received by at least one transition.

func checkMachineSignalCoverage(corpus *Corpus) []Finding {
	var findings []Finding
	for _, agentName := range corpus.MachineOrder {
		ms := corpus.Machines[agentName]
		received := make(map[string]bool)
		for _, tr := range ms.Transitions {
			received[tr.Signal] = true
		}
		for _, sig := range ms.Signals {
			if !received[sig.Name] {
				findings = append(findings, Finding{
					Check:   "machine-unreceived-signal",
					Level:   "warning",
					Message: fmt.Sprintf("machine %s signal %q is declared but no transition receives it", agentName, sig.Name),
				})
			}
		}
	}
	return findings
}

// checkMachineStateMetadata flags machines where some states have
// meaning annotations but others do not.

func checkMachineStateMetadata(corpus *Corpus) []Finding {
	var findings []Finding
	for _, agentName := range corpus.MachineOrder {
		ms := corpus.Machines[agentName]
		if len(ms.States) == 0 {
			continue
		}
		withMeaning := 0
		for _, st := range ms.States {
			if st.Meaning != "" {
				withMeaning++
			}
		}
		if withMeaning > 0 && withMeaning < len(ms.States) {
			var missing []string
			for _, st := range ms.States {
				if st.Meaning == "" {
					missing = append(missing, st.Name)
				}
			}
			findings = append(findings, Finding{
				Check:   "machine-incomplete-state-metadata",
				Level:   "warning",
				Message: fmt.Sprintf("machine %s has %d/%d states with meaning; missing: %s", agentName, withMeaning, len(ms.States), strings.Join(missing, ", ")),
			})
		}
	}
	return findings
}

// checkMachineSignalMetadata flags machines where some signals have
// trigger annotations but others do not.

func checkMachineSignalMetadata(corpus *Corpus) []Finding {
	var findings []Finding
	for _, agentName := range corpus.MachineOrder {
		ms := corpus.Machines[agentName]
		if len(ms.Signals) == 0 {
			continue
		}
		withTrigger := 0
		for _, sig := range ms.Signals {
			if sig.Trigger != "" {
				withTrigger++
			}
		}
		if withTrigger > 0 && withTrigger < len(ms.Signals) {
			var missing []string
			for _, sig := range ms.Signals {
				if sig.Trigger == "" {
					missing = append(missing, sig.Name)
				}
			}
			findings = append(findings, Finding{
				Check:   "machine-incomplete-signal-metadata",
				Level:   "warning",
				Message: fmt.Sprintf("machine %s has %d/%d signals with trigger; missing: %s", agentName, withTrigger, len(ms.Signals), strings.Join(missing, ", ")),
			})
		}
	}
	return findings
}

// checkToolSelectionDeclared verifies that every tool in a selection file
// has a corresponding declaration.

func checkMachineNameConsistency(corpus *Corpus) []Finding {
	var findings []Finding
	for _, agentName := range corpus.MachineOrder {
		ms := corpus.Machines[agentName]
		if ms.Name != agentName && !strings.HasPrefix(ms.Name, agentName+"-") {
			findings = append(findings, Finding{
				Check:   "machine-name-mismatch",
				Level:   "error",
				Message: fmt.Sprintf("machine %s directory name does not match spec name %q", agentName, ms.Name),
			})
		}
	}
	return findings
}

// checkUseCaseIndexRefs verifies that every use_case_index entry in
// SPECIFICATIONS.yaml references a use case that exists in the corpus.

func checkMachineDiagnostics(corpus *Corpus) []Finding {
	var findings []Finding
	for _, agentName := range corpus.MachineOrder {
		ms := corpus.Machines[agentName]
		for _, diag := range core.DiagnoseMachineSpec(ms) {
			level := "warning"
			if diag.Severity == core.MachineDiagnosticWarning {
				level = "warning"
			}
			findings = append(findings, Finding{
				Check:   "machine-diagnostic-" + diag.Code,
				Level:   level,
				Message: fmt.Sprintf("machine %s: %s", agentName, diag.Message),
			})
		}
	}
	return findings
}

// Errors returns only error-level findings.
