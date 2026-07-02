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

func (noopCmd) Name() string         { return "noop" }
func (noopCmd) Execute() core.Result { return core.Result{Signal: core.ToolDone} }
func (noopCmd) Undo() core.Result    { return core.NoopUndo("noop") }

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

func TestRegisterUnifiedToolsUnknownInit(t *testing.T) {
	t.Parallel()
	err := RegisterUnifiedTools(core.NewRegistry(), NewBuiltinRegistry(), "/tmp", []catalog.ToolDef{
		{Name: "bad", Type: "builtin", Init: "missing"},
	}, nil, func(catalog.ToolDef, string) core.Builder { return noopBuilder{} })

	require.ErrorContains(t, err, "unknown init")
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
