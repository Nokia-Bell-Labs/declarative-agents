// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"fmt"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// ValidateToolPhases rejects phase-scoped words that name no state in the
// loaded machine. Without this gate a typo registers successfully but makes the
// word invisible in every manifest and dynamic-dispatch path.
func ValidateToolPhases(machine core.MachineSpec, defs []ToolDef) error {
	states := make(map[string]bool, len(machine.States))
	for _, state := range machine.States {
		states[state.Name] = true
	}
	var failures []string
	for _, def := range defs {
		for _, phase := range def.Phases {
			if !states[phase] {
				failures = append(failures, fmt.Sprintf(
					"tool %q declares unknown phase %q", def.Name, phase,
				))
			}
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("tool phase validation: %s", strings.Join(failures, "; "))
	}
	return nil
}
