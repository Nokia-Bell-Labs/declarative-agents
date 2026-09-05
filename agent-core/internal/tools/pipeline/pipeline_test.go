// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package pipeline

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/compose"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

func registryWithStages(t *testing.T) *toolregistry.BuiltinRegistry {
	t.Helper()
	br := toolregistry.NewBuiltinRegistry()
	compose.RegisterFactories(br)
	RegisterFactories(br)
	return br
}

func buildPipeline(t *testing.T, br *toolregistry.BuiltinRegistry, config map[string]interface{}) core.Builder {
	t.Helper()
	factory, ok := br.Resolve(InitPipeline)
	require.True(t, ok)
	builder, err := factory(catalog.ToolDef{
		Name: "marshal_candidates", Type: "builtin", Init: InitPipeline, Config: config,
	}, nil)
	require.NoError(t, err)
	return builder
}

func viewFrom(entries ...core.Entry) core.CommandStateView {
	for i := range entries {
		entries[i].Result.RedactionVersion = core.OutputRedactionVersion1
		entries[i].Result.RedactionStatus = core.OutputRedactionApplied
	}
	return core.NewCommandStateView(core.Execution(entries))
}

// AC1: a render/compose marshalling block runs as one word: the first stage
// reads the pipeline's previous Result, the second reads the first through
// the current-value selector, and the configured signal is the only one the
// machine sees.
func TestPipelineChainsStagesAndEmitsTheConfiguredSignal(t *testing.T) {
	builder := buildPipeline(t, registryWithStages(t), map[string]interface{}{
		"signal": "CandidatesComposed",
		"stages": []interface{}{
			map[string]interface{}{
				"name": "render_lines",
				"init": "render_each",
				"config": map[string]interface{}{
					"items":         "$.chunks",
					"item_template": `{{ json text }}`,
					"separator":     ",",
					"signal":        "Rendered",
				},
			},
			map[string]interface{}{
				"init": "compose",
				"config": map[string]interface{}{
					"template": `{"documents":[{{ lines }}]}`,
					"inputs":   map[string]interface{}{"lines": "$."},
					"signal":   "Composed",
				},
			},
		},
	})

	cmd := builder.Build(core.Result{Output: `{"chunks":[{"text":"alpha"},{"text":"beta"}]}`})
	result := cmd.Execute()
	require.NoError(t, result.Err)
	require.Equal(t, core.Signal("CandidatesComposed"), result.Signal)
	require.Equal(t, "marshal_candidates", result.CommandName)
	require.JSONEq(t, `{"documents":["alpha","beta"]}`, result.Output)
}

// AC2 and AC6: a stage resolves $from against the same command-state view the
// pipeline received, exactly as the standalone word would.
func TestPipelineStagesReadPriorStepsThroughTheInjectedView(t *testing.T) {
	builder := buildPipeline(t, registryWithStages(t), map[string]interface{}{
		"signal": "PromptComposed",
		"stages": []interface{}{
			map[string]interface{}{
				"init": "compose",
				"config": map[string]interface{}{
					"template": "Q: {{ question }} P: {{ prev }}",
					"inputs": map[string]interface{}{
						"question": "$from(declare_question).question",
						"prev":     "$.",
					},
					"signal": "Composed",
				},
			},
		},
	})

	cmd := builder.Build(core.Result{Output: "carried"})
	cmd.(core.CommandStateAware).SetCommandState(viewFrom(core.Entry{
		CommandName: "declare_question",
		Result:      core.ResultDigest{Output: `{"question":"what moved?"}`},
	}))
	result := cmd.Execute()
	require.NoError(t, result.Err)
	require.Equal(t, "Q: what moved? P: carried", result.Output)
}

// AC3: misconfiguration fails registration with the pipeline and stage named.
func TestPipelineMisconfigurationFailsRegistration(t *testing.T) {
	br := registryWithStages(t)
	factory, ok := br.Resolve(InitPipeline)
	require.True(t, ok)
	build := func(config map[string]interface{}) error {
		_, err := factory(catalog.ToolDef{
			Name: "broken", Type: "builtin", Init: InitPipeline, Config: config,
		}, nil)
		return err
	}
	stage := func(init string, config map[string]interface{}) []interface{} {
		return []interface{}{map[string]interface{}{"init": init, "config": config}}
	}

	err := build(map[string]interface{}{"signal": "Done", "stages": stage("rest_client_invoke", nil)})
	require.ErrorContains(t, err, "not a pipeline stage")
	require.ErrorContains(t, err, "broken")

	err = build(map[string]interface{}{"signal": "Done", "stages": stage("render_each", map[string]interface{}{
		"item_template": "x", "signal": "Rendered",
	})})
	require.ErrorContains(t, err, "stage 1-render_each")

	err = build(map[string]interface{}{"signal": "Done", "stages": []interface{}{}})
	require.ErrorContains(t, err, "at least one stage")

	err = build(map[string]interface{}{"stages": stage("compose", map[string]interface{}{
		"template": "x", "signal": "Composed",
	})})
	require.ErrorContains(t, err, "signal is required")
}

// AC4: a failing stage ends the pipeline as CommandError naming the stage;
// a degraded-but-successful stage flows through unconverted.
func TestPipelineStageFailureNamesTheStageAndDegradationFlowsThrough(t *testing.T) {
	br := registryWithStages(t)

	failing := buildPipeline(t, br, map[string]interface{}{
		"signal": "Done",
		"stages": []interface{}{
			map[string]interface{}{
				"name": "render_lines",
				"init": "render_each",
				"config": map[string]interface{}{
					"items":         "$.chunks",
					"item_template": "{{ chunk }}",
					"signal":        "Rendered",
				},
			},
			map[string]interface{}{
				"init": "compose",
				"config": map[string]interface{}{
					"template": "never runs", "signal": "Composed",
				},
			},
		},
	})
	result := failing.Build(core.Result{Output: `not json`}).Execute()
	require.Error(t, result.Err)
	require.Equal(t, core.CommandError, result.Signal)
	require.ErrorContains(t, result.Err, "pipeline marshal_candidates stage render_lines")

	degraded := buildPipeline(t, br, map[string]interface{}{
		"signal": "Done",
		"stages": []interface{}{
			map[string]interface{}{
				"init": "compose",
				"config": map[string]interface{}{
					"template": "C: {{ missing }}",
					"inputs":   map[string]interface{}{"missing": "$from(absent).path"},
					"signal":   "Composed",
				},
			},
		},
	})
	cmd := degraded.Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(viewFrom())
	degradedResult := cmd.Execute()
	require.NoError(t, degradedResult.Err)
	require.Equal(t, core.Signal("Done"), degradedResult.Signal)
	require.Equal(t, "C: ", degradedResult.Output)
}

// AC5: a pipeline declaration co-selects its stage inits, so the stage
// factory families register without a standalone word.
func TestSelectedBuiltinInitsCoSelectPipelineStages(t *testing.T) {
	selected := toolregistry.SelectedBuiltinInits([]catalog.ToolDef{{
		Name: "marshal", Type: "builtin", Init: InitPipeline,
		Config: map[string]interface{}{
			"signal": "Done",
			"stages": []interface{}{
				map[string]interface{}{"init": "render_each"},
				map[string]interface{}{"init": "parse_structured"},
			},
		},
	}})
	require.True(t, selected[InitPipeline])
	require.True(t, selected["render_each"])
	require.True(t, selected["parse_structured"])
}
