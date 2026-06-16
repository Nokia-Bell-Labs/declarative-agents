// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/catalog"
	toolregistry "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/registry"
)

func TestRegisterEvalFactoriesRegistersExpectedInits(t *testing.T) {
	t.Parallel()
	br := toolregistry.NewBuiltinRegistry()

	RegisterEvalFactories(br, EvalFactoryDeps{Ctx: context.Background()})

	require.ElementsMatch(t, expectedEvalFactoryNames(), br.Names())
}

func TestRegisterEvalFactoriesReusesSessionState(t *testing.T) {
	t.Parallel()
	br := toolregistry.NewBuiltinRegistry()
	RegisterEvalFactories(br, EvalFactoryDeps{Ctx: context.Background(), SuitePath: "suite.yaml"})

	parseBuilder := resolveEvalBuilder[*ParseSuiteConfigBuilder](t, br, "parse_suite_config")
	discoverBuilder := resolveEvalBuilder[*DiscoverSuiteSamplesBuilder](t, br, "discover_suite_samples")

	require.Same(t, parseBuilder.ES, discoverBuilder.ES)
	require.Equal(t, "suite.yaml", parseBuilder.ES.SuitePath)
}

func resolveEvalBuilder[T any](t *testing.T, br *toolregistry.BuiltinRegistry, name string) T {
	t.Helper()
	factory, ok := br.Resolve(name)
	require.True(t, ok)
	builder, err := factory(catalog.ToolDef{Name: name, Init: name}, nil)
	require.NoError(t, err)
	typed, ok := builder.(T)
	require.True(t, ok)
	return typed
}

func expectedEvalFactoryNames() []string {
	names := make([]string, 0)
	for _, spec := range evalSessionFactorySpecs() {
		names = append(names, spec.name)
	}
	for _, spec := range evalConfiguredFactorySpecs() {
		names = append(names, spec.name)
	}
	for _, spec := range evalPointFactorySpecs() {
		names = append(names, spec.name)
	}
	return names
}
