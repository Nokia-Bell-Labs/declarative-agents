// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package core

import (
	"strings"
	"testing"

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
