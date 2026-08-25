// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package filesystem

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

func TestRegisterFactoriesProbesExpectedInits(t *testing.T) {
	t.Parallel()

	want := []string{
		InitFileRead, InitFileWrite, InitFileEdit, InitFileFind, InitListResource, InitReadResource,
	}
	br := toolregistry.NewBuiltinRegistry()
	RegisterFactories(br)
	require.ElementsMatch(t, want, br.Names())

	require.ElementsMatch(t, want, catalogInits(t, "filesystem", toolregistry.StandardFactoryDeps{
		RegisterFilesystem: RegisterFactories,
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

func TestFileFindFactoryAppliesOutputCap(t *testing.T) {
	t.Parallel()

	br := toolregistry.NewBuiltinRegistry()
	RegisterFactories(br)
	factory, ok := br.Resolve(InitFileFind)
	require.True(t, ok)
	builder, err := factory(catalog.ToolDef{OutputCap: 17}, map[string]string{"directory": "/tmp"})
	require.NoError(t, err)
	require.Equal(t, 17, builder.(*FindBuilder).OutputLineCap)
}
