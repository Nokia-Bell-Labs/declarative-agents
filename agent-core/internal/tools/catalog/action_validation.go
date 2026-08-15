// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"fmt"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// ValidateMachineActions checks that every literal machine action is present in
// the profile-selected tool definitions. Empty actions and dynamic $tool
// dispatch do not name a selected ToolDef.
func ValidateMachineActions(spec core.MachineSpec, defs []ToolDef) error {
	selected := make(map[string]bool, len(defs))
	for _, def := range defs {
		selected[def.Name] = true
	}

	var errs []string
	for i, transition := range spec.Transitions {
		if transition.Action == "" || transition.Action == "$tool" || selected[transition.Action] {
			continue
		}
		errs = append(errs, fmt.Sprintf(
			"transition[%d] %s/%s action %q is not selected for machine %q",
			i, transition.State, transition.Signal, transition.Action, spec.Name,
		))
	}
	if len(errs) > 0 {
		return fmt.Errorf("machine action validation: %s", strings.Join(errs, "; "))
	}
	return nil
}
