// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type registryTestBuilder struct{}

func (registryTestBuilder) Build(Result) Command { return registryTestCmd{} }

type registryTestCmd struct{}

func (registryTestCmd) Name() string       { return "registry_test" }
func (registryTestCmd) Execute() Result    { return Result{Signal: ToolDone} }
func (registryTestCmd) Undo(Result) Result { return NoopUndo("registry_test") }

func TestResolveExternalToolClassifiesAvailability(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	builder := registryTestBuilder{}
	reg.Register(ToolSpec{Name: "read", Visibility: External, Phases: []State{"Composing"}, PhaseScoped: true}, builder)
	reg.Register(ToolSpec{Name: "write", Visibility: External, Phases: []State{"Reviewing"}, PhaseScoped: true}, builder)
	reg.Register(ToolSpec{Name: "hidden", Visibility: Internal}, builder)

	spec, gotBuilder, availability := reg.ResolveExternalTool("read", "Composing")
	require.Equal(t, ExternalToolAvailable, availability)
	require.Equal(t, "read", spec.Name)
	require.Equal(t, builder, gotBuilder)

	_, _, availability = reg.ResolveExternalTool("write", "Composing")
	require.Equal(t, ExternalToolUnavailableInState, availability)

	_, _, availability = reg.ResolveExternalTool("hidden", "Composing")
	require.Equal(t, ExternalToolInternal, availability)

	_, _, availability = reg.ResolveExternalTool("missing", "Composing")
	require.Equal(t, ExternalToolUnknown, availability)
}

func TestManifestAndAvailableExternalToolNamesUseAvailabilityRule(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	reg.Register(ToolSpec{Name: "read", Visibility: External, Phases: []State{"Composing"}, PhaseScoped: true}, registryTestBuilder{})
	reg.Register(ToolSpec{Name: "write", Visibility: External, Phases: []State{"Reviewing"}, PhaseScoped: true}, registryTestBuilder{})
	reg.Register(ToolSpec{Name: "global", Visibility: External}, registryTestBuilder{})
	reg.Register(ToolSpec{Name: "hidden", Visibility: Internal}, registryTestBuilder{})

	manifest := reg.Manifest("Composing")
	require.Len(t, manifest, 2)
	require.Equal(t, "global", manifest[0].Name)
	require.Equal(t, "read", manifest[1].Name)
	require.Equal(t, []string{"global", "read"}, reg.AvailableExternalToolNames("Composing"))
}

func TestRegistryCheckedRegistrationReturnsConfigurationErrors(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()

	require.ErrorContains(t, reg.RegisterChecked(ToolSpec{}, registryTestBuilder{}), "ToolSpec.Name must not be empty")
	require.NoError(t, reg.RegisterChecked(ToolSpec{Name: "read"}, registryTestBuilder{}))
	require.ErrorContains(t, reg.RegisterChecked(ToolSpec{Name: "read"}, registryTestBuilder{}), `duplicate tool name "read"`)
	require.ErrorContains(t, reg.OverrideChecked(ToolSpec{}, registryTestBuilder{}), "ToolSpec.Name must not be empty")

	reg.Freeze()
	require.ErrorContains(t, reg.RegisterChecked(ToolSpec{Name: "write"}, registryTestBuilder{}), "Register called after Freeze")
	require.ErrorContains(t, reg.OverrideChecked(ToolSpec{Name: "write"}, registryTestBuilder{}), "Override called after Freeze")
}

func TestRegistryCompatibilityWrappersStillPanic(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	require.PanicsWithValue(t, "registry: ToolSpec.Name must not be empty", func() {
		reg.Register(ToolSpec{}, registryTestBuilder{})
	})

	require.NoError(t, reg.RegisterChecked(ToolSpec{Name: "read"}, registryTestBuilder{}))
	require.PanicsWithValue(t, `registry: duplicate tool name "read"`, func() {
		reg.Register(ToolSpec{Name: "read"}, registryTestBuilder{})
	})

	reg.Freeze()
	require.PanicsWithValue(t, "registry: Override called after Freeze", func() {
		reg.Override(ToolSpec{Name: "read"}, registryTestBuilder{})
	})
}
