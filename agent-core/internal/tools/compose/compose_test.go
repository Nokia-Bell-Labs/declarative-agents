// Copyright (c) 2026 Nokia. All rights reserved.

package compose

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func viewFrom(entries ...core.Entry) core.CommandStateView {
	return core.NewCommandStateView(core.Execution(entries))
}

func TestComposeRendersFromSelectors(t *testing.T) {
	cmd := Builder{
		ToolName: "compose_prompt",
		Template: "Q: {{ message }}\nCtx: {{ chunks }}",
		Inputs: map[string]string{
			"message": "$from(embed).carried.input",
			"chunks":  "$from(rag).mapped.documents",
		},
		Signal: "Composed",
	}.Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(viewFrom(
		core.Entry{CommandName: "embed", Result: core.ResultDigest{Output: `{"carried":{"input":"what is x?"}}`}},
		core.Entry{CommandName: "rag", Result: core.ResultDigest{Output: `{"mapped":{"documents":["chunk-a","chunk-b"]}}`}},
	))

	res := cmd.Execute()
	require.Equal(t, core.Signal("Composed"), res.Signal)
	require.NoError(t, res.Err)
	require.Contains(t, res.Output, "Q: what is x?")
	require.Contains(t, res.Output, `Ctx: ["chunk-a","chunk-b"]`)
}

func TestComposeMissingSelectorRendersEmptyAndReports(t *testing.T) {
	cmd := Builder{
		ToolName: "compose_prompt",
		Template: "Q: {{ message }}|C: {{ chunks }}",
		Inputs: map[string]string{
			"message": "$from(embed).carried.input",
			"chunks":  "$from(rag).mapped.documents",
		},
	}.Build(core.Result{})
	// rag step is absent, so the chunks selector does not resolve.
	cmd.(core.CommandStateAware).SetCommandState(viewFrom(
		core.Entry{CommandName: "embed", Result: core.ResultDigest{Output: `{"carried":{"input":"hi"}}`}},
	))

	res := cmd.Execute()
	require.Equal(t, core.Signal("Composed"), res.Signal, "default signal renders even with a degraded input")
	require.Error(t, res.Err, "the unresolved selector is reported")
	require.Contains(t, res.Output, "Q: hi|C: ", "the missing chunk renders empty")
	require.Contains(t, res.Err.Error(), "chunks")
}

func TestComposeMostRecentWins(t *testing.T) {
	cmd := Builder{
		ToolName: "c",
		Template: "{{ v }}",
		Inputs:   map[string]string{"v": "$from(step).mapped.value"},
	}.Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(viewFrom(
		core.Entry{CommandName: "step", Result: core.ResultDigest{Output: `{"mapped":{"value":"first"}}`}},
		core.Entry{CommandName: "step", Result: core.ResultDigest{Output: `{"mapped":{"value":"second"}}`}},
	))
	require.Equal(t, "second", cmd.Execute().Output)
}

func TestComposeNoViewRendersEmptyAndReports(t *testing.T) {
	cmd := Builder{
		ToolName: "c",
		Template: "[{{ v }}]",
		Inputs:   map[string]string{"v": "$from(x).a.b"},
	}.Build(core.Result{})
	// No SetCommandState: the view is nil.
	res := cmd.Execute()
	require.Error(t, res.Err)
	require.Equal(t, "[]", res.Output)
}
