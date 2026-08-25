// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package validation

import (
	"testing"

	"github.com/stretchr/testify/require"

	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

func TestRegisterSpecFactoriesProbesExpectedInits(t *testing.T) {
	t.Parallel()

	br := toolregistry.NewBuiltinRegistry()
	RegisterSpecFactories(br, FactoryDeps{})
	require.Equal(t, br.Names(), catalogInits(t, "spec_validation", toolregistry.StandardFactoryDeps{
		RegisterSpecValidation: func(reg *toolregistry.BuiltinRegistry) {
			RegisterSpecFactories(reg, FactoryDeps{})
		},
	}))
}

func catalogInits(t *testing.T, family string, deps toolregistry.StandardFactoryDeps) []string {
	t.Helper()
	for _, entry := range toolregistry.StandardFactoryCatalog(deps) {
		if entry.Name == family {
			return entry.Inits
		}
	}
	t.Fatalf("standard catalog missing family %q", family)
	return nil
}
