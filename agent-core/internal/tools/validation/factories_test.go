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

// A relative charter path names a file in the audited module, so it resolves
// against the directory var rather than the process working directory; the
// catalog's gate used to work only because mage happened to run in that
// module (GH-2346). Absolute paths, including /opt/agent-core installs, keep
// their meaning.
func TestRelativeSuitePathsResolveAgainstTheAuditedDirectory(t *testing.T) {
	t.Parallel()

	resolved := resolveSuitePaths(
		[]string{"docs/corpus-charter.yaml", "/opt/agent-core/tools/charter.yaml"},
		"/modules/catalog",
	)
	require.Equal(t, []string{
		"/modules/catalog/docs/corpus-charter.yaml",
		"/opt/agent-core/tools/charter.yaml",
	}, resolved)
}

func TestSuitePathsWithoutADirectoryVarStayUntouched(t *testing.T) {
	t.Parallel()

	paths := []string{"docs/corpus-charter.yaml"}
	require.Equal(t, paths, resolveSuitePaths(paths, ""))
}
