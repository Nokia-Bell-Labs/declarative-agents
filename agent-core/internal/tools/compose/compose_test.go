// Copyright (c) 2026 Nokia. All rights reserved.

package compose

import (
	"encoding/json"
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

// TestComposeRendersResolvableJSONObject pins the contract the chatbot's
// exclusion path depends on (GH-765): a compose word whose template renders a
// JSON object republishes values that a later word can address with
// $from(label).path. ResolveFromSelector requires the referenced step's output
// to be a JSON object, so a template that renders a bare scalar or array would
// not be addressable and the gate would silently resolve nothing.
func TestComposeRendersResolvableJSONObject(t *testing.T) {
	keep := Builder{
		ToolName: "keep_chunks0",
		Template: `{"documents": {{ documents }}}`,
		Inputs:   map[string]string{"documents": "$from(rag_query0).mapped.documents"},
		Signal:   "ChunksKept0",
	}.Build(core.Result{})
	keep.(core.CommandStateAware).SetCommandState(viewFrom(
		core.Entry{CommandName: "rag_query0", Result: core.ResultDigest{
			Output: `{"mapped":{"documents":["chunk about the rig","chunk about kind"]}}`}},
	))

	res := keep.Execute()
	require.NoError(t, res.Err)
	require.Equal(t, core.Signal("ChunksKept0"), res.Signal)

	// The rendered output must itself be addressable by a later word.
	view := viewFrom(core.Entry{CommandName: "keep_chunks0", Result: core.ResultDigest{Output: res.Output}})
	value, err := core.ResolveFromSelector(view, "$from(keep_chunks0).documents")
	require.NoError(t, err)
	require.Equal(t, []interface{}{"chunk about the rig", "chunk about kind"}, value)
}

// TestComposeRendersResolvableConstantObject pins the same contract for a word
// that renders a configured constant with no inputs, which is how the chatbot
// puts its query embedding model identity into command state (GH-765).
func TestComposeRendersResolvableConstantObject(t *testing.T) {
	declare := Builder{
		ToolName: "declare_query_model",
		Template: `{"model": "qwen3-embedding:8b"}`,
		Signal:   "QueryModelDeclared",
	}.Build(core.Result{})
	declare.(core.CommandStateAware).SetCommandState(viewFrom())

	res := declare.Execute()
	require.NoError(t, res.Err)

	view := viewFrom(core.Entry{CommandName: "declare_query_model", Result: core.ResultDigest{Output: res.Output}})
	value, err := core.ResolveFromSelector(view, "$from(declare_query_model).model")
	require.NoError(t, err)
	require.Equal(t, "qwen3-embedding:8b", value)
}

// TestComposeJSONSubstitutionEscapes proves the {{ json key }} form renders a
// document that survives a value carrying quotes, backslashes, and newlines.
// The raw form does not, which is why the json form exists (GH-766): an LLM
// answer routinely contains all three.
func TestComposeJSONSubstitutionEscapes(t *testing.T) {
	answer := "He said \"hi\"\nand left a \\ behind"
	build := func(template string) core.Command {
		cmd := Builder{
			ToolName: "compose_response",
			Template: template,
			Inputs:   map[string]string{"answer": "$from(answer_word).mapped.text"},
		}.Build(core.Result{})
		payload, err := json.Marshal(map[string]any{"mapped": map[string]any{"text": answer}})
		require.NoError(t, err)
		cmd.(core.CommandStateAware).SetCommandState(viewFrom(
			core.Entry{CommandName: "answer_word", Result: core.ResultDigest{Output: string(payload)}},
		))
		return cmd
	}

	encoded := build(`{"answer": {{ json answer }}}`).Execute()
	require.NoError(t, encoded.Err)
	var decoded map[string]string
	require.NoError(t, json.Unmarshal([]byte(encoded.Output), &decoded),
		"json substitution must render a parseable document: %s", encoded.Output)
	require.Equal(t, answer, decoded["answer"], "the value must survive the round trip intact")

	raw := build(`{"answer": "{{ answer }}"}`).Execute()
	require.Error(t, json.Unmarshal([]byte(raw.Output), &decoded),
		"raw substitution of the same value should not parse; if it does, this test no longer proves why the json form is needed")
}

// TestComposeJSONSubstitutionUnresolved proves an unresolved selector encodes
// to an empty JSON string rather than leaving a hole, so a degraded or excluded
// source still yields a parseable document.
func TestComposeJSONSubstitutionUnresolved(t *testing.T) {
	cmd := Builder{
		ToolName: "compose_response",
		Template: `{"outcome": {{ json outcome }}, "chunks": {{ json chunks }}}`,
		Inputs: map[string]string{
			"outcome": "$from(missing).outcome",
			"chunks":  "$from(kept).documents",
		},
	}.Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(viewFrom(
		core.Entry{CommandName: "kept", Result: core.ResultDigest{Output: `{"documents":["a","b"]}`}},
	))

	res := cmd.Execute()
	require.Error(t, res.Err, "the unresolved selector is still reported")

	var decoded struct {
		Outcome string   `json:"outcome"`
		Chunks  []string `json:"chunks"`
	}
	require.NoError(t, json.Unmarshal([]byte(res.Output), &decoded),
		"an unresolved selector must not break the document: %s", res.Output)
	require.Equal(t, "", decoded.Outcome)
	require.Equal(t, []string{"a", "b"}, decoded.Chunks)
}

// TestComposeRawFormUnchanged proves the raw form still renders as before, so
// the prompt templates that depend on it are unaffected.
func TestComposeRawFormUnchanged(t *testing.T) {
	cmd := Builder{
		ToolName: "compose_prompt",
		Template: "Q: {{ message }}\nCtx: {{ chunks }}",
		Inputs: map[string]string{
			"message": "$from(embed).carried.input",
			"chunks":  "$from(rag).mapped.documents",
		},
	}.Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(viewFrom(
		core.Entry{CommandName: "embed", Result: core.ResultDigest{Output: `{"carried":{"input":"what is x?"}}`}},
		core.Entry{CommandName: "rag", Result: core.ResultDigest{Output: `{"mapped":{"documents":["chunk-a"]}}`}},
	))

	res := cmd.Execute()
	require.NoError(t, res.Err)
	require.Contains(t, res.Output, "Q: what is x?")
	require.Contains(t, res.Output, `Ctx: ["chunk-a"]`)
}
