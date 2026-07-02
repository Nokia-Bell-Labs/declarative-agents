// Copyright (c) 2026 Nokia. All rights reserved.

package registry

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStandardFactoryCatalogSelectsEntriesByInit(t *testing.T) {
	t.Parallel()
	entries := StandardFactoryCatalog(StandardFactoryDeps{})
	byName := make(map[string]StandardFactoryCatalogEntry, len(entries))
	for _, entry := range entries {
		byName[entry.Name] = entry
	}

	require.True(t, byName["planning"].SelectedBy(map[string]bool{"execute_task": true}))
	require.True(t, byName["evaluation"].SelectedBy(map[string]bool{"run_point": true}))
	require.True(t, byName["bench"].SelectedBy(map[string]bool{"launch_eval": true}))
	require.True(t, byName["spec_validation"].SelectedBy(map[string]bool{"validate_specs": true}))
	require.True(t, byName["lifecycle"].SelectedBy(map[string]bool{"checkpoint_history": true}))
	require.False(t, byName["planning"].SelectedBy(map[string]bool{"launch_eval": true}))
}

func TestRegisterStandardBuiltinFactoriesGatesBySelectedInit(t *testing.T) {
	t.Parallel()
	br := NewBuiltinRegistry()
	var planningCalled bool
	var benchCalled bool
	deps := StandardFactoryDeps{
		RegisterPlanning: func(*BuiltinRegistry) { planningCalled = true },
		RegisterBench:    func(*BuiltinRegistry) { benchCalled = true },
	}

	RegisterStandardBuiltinFactories(br, map[string]bool{"execute_task": true}, deps)

	require.True(t, planningCalled)
	require.False(t, benchCalled)
}
