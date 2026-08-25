// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package dolt

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

func TestRegisterFactoriesProbesExpectedInits(t *testing.T) {
	t.Parallel()

	want := []string{InitProvision, InitQuery, InitWrite}
	br := toolregistry.NewBuiltinRegistry()
	RegisterFactories(br, FactoryDeps{})
	require.ElementsMatch(t, want, br.Names())

	require.ElementsMatch(t, want, catalogInits(t, "dolt", toolregistry.StandardFactoryDeps{
		RegisterDolt: func(br *toolregistry.BuiltinRegistry) { RegisterFactories(br, FactoryDeps{}) },
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

func TestRegisterFactoriesDefersIdentityErrorUntilBuild(t *testing.T) {
	t.Parallel()

	identityErr := errors.New("resolve active Dolt checkpoint identity: parse Dolt connection")
	br := toolregistry.NewBuiltinRegistry()
	require.NotPanics(t, func() {
		RegisterFactories(br, FactoryDeps{CheckpointIdentityErr: identityErr})
	})
	require.ElementsMatch(t, []string{InitProvision, InitQuery, InitWrite}, br.Names())

	factory, ok := br.Resolve(InitQuery)
	require.True(t, ok)
	_, err := factory(catalog.ToolDef{Name: "lookup_records"}, nil)
	require.ErrorIs(t, err, identityErr)
}
