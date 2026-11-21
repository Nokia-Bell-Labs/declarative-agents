// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package core

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestSpecPinMachineBodyLoadsWithoutName pins R-MACH-001 as the loader
// enforces it: a machine-profile body needs states and transitions, not a
// name, so a nameless body loads.
func TestSpecPinMachineBodyLoadsWithoutName(t *testing.T) {
	t.Parallel()
	nameless := strings.Replace(validYAML, "name: test-machine\n", "", 1)
	require.NotContains(t, nameless, "name: test-machine")
	_, err := ParseMachineSpec([]byte(nameless))
	require.NoError(t, err)
}

// TestSpecPinMachineRejectsAnUnparseableBudgetDuration pins R-MACH-008's
// duration rule: a budget duration that is present must parse as a positive
// duration, so a typo cannot silently drop the run's wall-clock bound.
func TestSpecPinMachineRejectsAnUnparseableBudgetDuration(t *testing.T) {
	t.Parallel()
	const budget = "budget: {max_iterations: 100, command_timeout: 1m}"
	require.Contains(t, validYAML, budget)
	for name, tc := range map[string]struct{ replacement, wantErr string }{
		"max_duration typo": {
			"budget: {max_iterations: 100, command_timeout: 1m, max_duration: 10 minutes}",
			`budget.max_duration: invalid positive duration "10 minutes"`,
		},
		"max_duration zero": {
			"budget: {max_iterations: 100, command_timeout: 1m, max_duration: 0s}",
			`budget.max_duration: invalid positive duration "0s"`,
		},
		"command_timeout typo": {
			"budget: {max_iterations: 100, command_timeout: one minute}",
			`budget.command_timeout: invalid positive duration "one minute"`,
		},
	} {
		_, err := ParseMachineSpec([]byte(strings.Replace(validYAML, budget, tc.replacement, 1)))
		require.ErrorContains(t, err, tc.wantErr, name)
	}
	spec, err := ParseMachineSpec([]byte(strings.Replace(validYAML, budget,
		"budget: {max_iterations: 100, command_timeout: 1m, max_duration: 10m}", 1)))
	require.NoError(t, err)
	require.Equal(t, 10*time.Minute, spec.BudgetSpec.ToBudget(Budget{}).MaxDuration)
}
