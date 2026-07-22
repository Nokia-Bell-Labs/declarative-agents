// Copyright (c) 2026 Nokia. All rights reserved.

package registry

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

type noopBuilder struct{}

func (noopBuilder) Build(_ core.Result) core.Command { return noopCmd{} }

type noopCmd struct{}

func (noopCmd) Name() string                   { return "noop" }
func (noopCmd) Execute() core.Result           { return core.Result{Signal: core.ToolDone} }
func (noopCmd) Undo(_ core.Result) core.Result { return core.NoopUndo("noop") }

func TestBuiltinRegistryRegisterResolveOverride(t *testing.T) {
	t.Parallel()
	br := NewBuiltinRegistry()
	br.Register("init", func(catalog.ToolDef, map[string]string) (core.Builder, error) {
		return noopBuilder{}, nil
	})
	_, ok := br.Resolve("init")
	require.True(t, ok)

	br.Override("init", func(catalog.ToolDef, map[string]string) (core.Builder, error) {
		return noopBuilder{}, nil
	})
	_, ok = br.Resolve("init")
	require.True(t, ok)
	require.Contains(t, br.Names(), "init")
}

func TestBuiltinRegistryDuplicatePanics(t *testing.T) {
	t.Parallel()
	br := NewBuiltinRegistry()
	br.Register("dup", func(catalog.ToolDef, map[string]string) (core.Builder, error) {
		return noopBuilder{}, nil
	})
	require.Panics(t, func() {
		br.Register("dup", func(catalog.ToolDef, map[string]string) (core.Builder, error) {
			return noopBuilder{}, nil
		})
	})
}

func TestRegisterUnifiedToolsBuiltinAndExec(t *testing.T) {
	t.Parallel()
	reg := core.NewRegistry()
	br := NewBuiltinRegistry()
	br.Register("file_read", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return noopBuilder{}, nil
	})
	defs := []catalog.ToolDef{
		{Name: "stage_all", Type: "exec", Binary: "git"},
		{Name: "read", Type: "builtin", Init: "file_read"},
	}

	err := RegisterUnifiedTools(reg, br, "/tmp", defs, nil, func(catalog.ToolDef, string) core.Builder {
		return noopBuilder{}
	})

	require.NoError(t, err)
	_, ok := reg.Resolve("stage_all")
	require.True(t, ok)
	_, ok = reg.Resolve("read")
	require.True(t, ok)
}

func TestRegisterUnifiedToolsForMachineAppliesDynamicPhases(t *testing.T) {
	t.Parallel()
	reg := core.NewRegistry()
	br := NewBuiltinRegistry()
	for _, initName := range []string{"file_read", "broken", "internal"} {
		br.Register(initName, func(catalog.ToolDef, map[string]string) (core.Builder, error) {
			return noopBuilder{}, nil
		})
	}
	machine := core.MachineSpec{
		Name:           "dynamic",
		States:         core.StateSpecsFromNames("Parsing", "Composing", "Done", "Failed"),
		TerminalStates: []string{"Done", "Failed"},
		Signals:        core.SignalSpecsFromNames("ToolReady", "ToolDone", "ToolFailed", "MissingRoute"),
		Transitions: []core.TransitionSpec{
			{State: "Parsing", Signal: "ToolReady", Next: "Composing", Action: "$tool"},
			{State: "Composing", Signal: "ToolDone", Next: "Composing"},
			{State: "Composing", Signal: "ToolFailed", Next: "Failed"},
		},
	}
	defs := []catalog.ToolDef{
		{Name: "read", Type: "builtin", Init: "file_read", Emits: []string{"ToolDone", "ToolFailed"}},
		{Name: "broken", Type: "builtin", Init: "broken", Emits: []string{"MissingRoute"}},
		{Name: "parse_response", Type: "builtin", Init: "internal", Visibility: "internal", Emits: []string{"ToolDone"}},
	}

	err := RegisterUnifiedToolsForMachine(reg, br, "/tmp", machine, defs, nil, func(catalog.ToolDef, string) core.Builder {
		return noopBuilder{}
	})

	require.NoError(t, err)
	requireManifestNames(t, reg.Manifest("Composing"), []string{"read"})
	requireManifestNames(t, reg.Manifest("Parsing"), []string{})
	_, ok := reg.Resolve("broken")
	require.True(t, ok, "tools hidden from dynamic manifests still resolve for named actions")
}

func TestRegisterUnifiedToolsUnknownInit(t *testing.T) {
	t.Parallel()
	err := RegisterUnifiedTools(core.NewRegistry(), NewBuiltinRegistry(), "/tmp", []catalog.ToolDef{
		{Name: "bad", Type: "builtin", Init: "missing"},
	}, nil, func(catalog.ToolDef, string) core.Builder { return noopBuilder{} })

	require.ErrorContains(t, err, "unknown init")
}

func TestRegisterUnifiedToolsDuplicateReturnsRegistryError(t *testing.T) {
	t.Parallel()
	br := NewBuiltinRegistry()
	br.Register("file_read", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return noopBuilder{}, nil
	})

	err := RegisterUnifiedTools(core.NewRegistry(), br, "/tmp", []catalog.ToolDef{
		{Name: "read", Type: "builtin", Init: "file_read"},
		{Name: "read", Type: "builtin", Init: "file_read"},
	}, nil, func(catalog.ToolDef, string) core.Builder { return noopBuilder{} })

	require.ErrorContains(t, err, `registry: duplicate tool name "read"`)
}

func TestRegisterSingleBuiltinUnknownInitMatchesUnifiedPath(t *testing.T) {
	t.Parallel()
	td := catalog.ToolDef{Name: "bad", Type: "builtin", Init: "missing"}

	singleErr := RegisterSingleBuiltin(core.NewRegistry(), NewBuiltinRegistry(), td, nil)
	unifiedErr := RegisterUnifiedTools(core.NewRegistry(), NewBuiltinRegistry(), "/tmp", []catalog.ToolDef{td}, nil, func(catalog.ToolDef, string) core.Builder {
		return noopBuilder{}
	})

	require.Error(t, singleErr)
	require.Error(t, unifiedErr)
	require.Equal(t, singleErr.Error(), unifiedErr.Error(), "single and unified registration must report the same contract failure")
	require.ErrorContains(t, singleErr, `builtin tool "bad"`)
	require.ErrorContains(t, singleErr, `unknown init "missing"`)
}

func TestRegisterSingleBuiltinOverrides(t *testing.T) {
	t.Parallel()
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{Name: "read"}, noopBuilder{})
	br := NewBuiltinRegistry()
	br.Register("file_read", func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		return noopBuilder{}, nil
	})

	err := RegisterSingleBuiltin(reg, br, catalog.ToolDef{Name: "read", Type: "builtin", Init: "file_read"}, nil)

	require.NoError(t, err)
	_, ok := reg.Resolve("read")
	require.True(t, ok)
}

func TestSelectedBuiltinInits(t *testing.T) {
	t.Parallel()
	selected := SelectedBuiltinInits([]catalog.ToolDef{
		{Name: "read", Type: "builtin", Init: "file_read"},
		{Name: "build", Type: "exec", Binary: "go"},
		{Name: "parse_response", Type: "builtin", Init: "parse_response"},
	})

	require.True(t, selected["file_read"])
	require.True(t, selected["parse_response"])
	require.False(t, selected["build"])
}

func requireManifestNames(t *testing.T, specs []core.ToolSpec, want []string) {
	t.Helper()
	got := make([]string, 0, len(specs))
	for _, spec := range specs {
		got = append(got, spec.Name)
	}
	require.Equal(t, want, got)
}
