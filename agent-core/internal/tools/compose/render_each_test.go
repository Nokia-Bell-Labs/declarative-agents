// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package compose

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestRenderEachRendersInOrder(t *testing.T) {
	cmd := RenderEachBuilder{
		ToolName: "render_each", Items: "$from(selected).matched",
		ItemTemplate: "[{{ name }}]\n{{ json values }}", Separator: "\n\n", Signal: "Rendered",
	}.Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(viewFrom(core.Entry{
		CommandName: "selected",
		Result:      core.ResultDigest{Output: `{"matched":[{"name":"a","values":[1,2]},{"name":"b","values":[]}]}`},
	}))

	res := cmd.Execute()
	require.NoError(t, res.Err)
	require.Equal(t, core.Signal("Rendered"), res.Signal)
	require.Equal(t, "[a]\n[1,2]\n\n[b]\n[]", res.Output)
}

func TestRenderEachTraversesIteratorStructuredOutput(t *testing.T) {
	cmd := RenderEachBuilder{
		ToolName: "render_each", Items: "$from(join).items",
		ItemTemplate: "{{ result.structured_output.mapped.name }}",
		Separator:    ",", Signal: "Rendered",
	}.Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(viewFrom(core.Entry{
		CommandName: "join",
		Result: core.ResultDigest{Output: `{"items":[
			{"result":{"output":"{\"mapped\":{\"name\":\"a\"}}","structured_output":{"mapped":{"name":"a"}}}},
			{"result":{"output":"{\"mapped\":{\"name\":\"b\"}}","structured_output":{"mapped":{"name":"b"}}}}
		]}`},
	}))
	res := cmd.Execute()
	require.NoError(t, res.Err)
	require.Equal(t, "a,b", res.Output)
}

func TestRenderEachEmptyArrayRendersEmpty(t *testing.T) {
	cmd := RenderEachBuilder{
		ToolName: "render_each", Items: "$from(selected).matched",
		ItemTemplate: "{{ name }}", Separator: ",", Signal: "Rendered",
	}.Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(viewFrom(core.Entry{
		CommandName: "selected", Result: core.ResultDigest{Output: `{"matched":[]}`},
	}))
	res := cmd.Execute()
	require.NoError(t, res.Err)
	require.Equal(t, core.Signal("Rendered"), res.Signal)
	require.Empty(t, res.Output)
}

func TestRenderEachUnresolvedSelectorIsCommandError(t *testing.T) {
	cmd := RenderEachBuilder{
		ToolName: "render_each", Items: "$from(missing).items",
		ItemTemplate: "{{ name }}", Signal: "Rendered",
	}.Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(viewFrom())
	res := cmd.Execute()
	require.Equal(t, core.CommandError, res.Signal)
	var unresolved *core.UnresolvedLabelError
	require.True(t, errors.As(res.Err, &unresolved))
}

func TestValidateRenderEachConfigRejectsMalformedConfig(t *testing.T) {
	require.Error(t, ValidateRenderEachConfig("r", "$.items", "{{ name }}", "Done"))
	require.Error(t, ValidateRenderEachConfig("r", "$from(x).items", "", "Done"))
	require.Error(t, ValidateRenderEachConfig("r", "$from(x).items", "{{ bad path }}", "Done"))
	require.Error(t, ValidateRenderEachConfig("r", "$from(x).items", "{{ name }}", ""))
	require.NoError(t, ValidateRenderEachConfig("r", "$from(x).items", "{{ name }}", "Done"))
}
