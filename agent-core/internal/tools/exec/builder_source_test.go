// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package exec

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

// Parameters a declaration sources from command state rather than the
// previous result (srd023 R2.8, srd038).

func TestExecBuilderCommandStateSourceAcrossInterveningStep(t *testing.T) {
	td := catalog.ToolDef{
		Name:   "run_corpus_ingest",
		Binary: "printf",
		Args:   []string{"%s|%s"},
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"directory": map[string]interface{}{
					"type": "string", "positional": true, "position": 1,
					"source": "$from(seed).parameters.directory",
				},
				"mode": map[string]interface{}{
					"type": "string", "positional": true, "position": 2,
				},
			},
			"required": []interface{}{"directory", "mode"},
		},
	}

	mappings := td.ExtractParamMappings()
	directory := findParamMapping(t, mappings, "directory")
	require.Equal(t, "$from(seed).parameters.directory", directory.Source)

	var schema map[string]interface{}
	require.NoError(t, json.Unmarshal(td.ToToolSpec().InputSchema, &schema))
	properties := schema["properties"].(map[string]interface{})
	require.NotContains(t, properties["directory"].(map[string]interface{}), "source")

	cmd := (&ExecBuilder{Def: td, Root: t.TempDir()}).Build(core.Result{
		Output: `{"parameters":{"mode":"full"}}`,
	})
	aware, ok := cmd.(core.CommandStateAware)
	require.True(t, ok)
	aware.SetCommandState(core.NewCommandStateView(core.Execution{
		{
			CommandName: "seed",
			Label:       "seed",
			Result: core.ResultDigest{
				Output:           `{"parameters":{"directory":"/tmp/corpus"}}`,
				RedactionVersion: core.OutputRedactionVersion1,
				RedactionStatus:  core.OutputRedactionApplied,
			},
		},
		{
			CommandName: "count_before",
			Label:       "count_before",
			Result: core.ResultDigest{
				Output:           `{"mapped":{"count":"4"}}`,
				RedactionVersion: core.OutputRedactionVersion1,
				RedactionStatus:  core.OutputRedactionApplied,
			},
		},
	}))

	result := cmd.Execute()

	require.Equal(t, core.ToolDone, result.Signal, result.Output)
	require.Equal(t, "/tmp/corpus|full", result.Output)
}

func TestExecBuilderCommandStateSourceFailures(t *testing.T) {
	t.Run("missing label preserves typed cause and does not launch", func(t *testing.T) {
		marker := t.TempDir() + "/launched"
		cmd := sourceTouchBuilder(marker, "$from(absent).parameters.directory").Build(core.Result{})
		cmd.(core.CommandStateAware).SetCommandState(core.NewCommandStateView(nil))

		result := cmd.Execute()

		require.Equal(t, core.CommandError, result.Signal)
		var target *core.UnresolvedLabelError
		require.ErrorAs(t, result.Err, &target)
		require.Equal(t, "absent", target.Label)
		_, err := os.Stat(marker)
		require.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("missing path preserves typed cause and does not launch", func(t *testing.T) {
		marker := t.TempDir() + "/launched"
		cmd := sourceTouchBuilder(marker, "$from(seed).parameters.absent").Build(core.Result{})
		cmd.(core.CommandStateAware).SetCommandState(core.NewCommandStateView(core.Execution{{
			CommandName: "seed",
			Label:       "seed",
			Result: core.ResultDigest{
				Output:           `{"parameters":{"directory":"/tmp/corpus"}}`,
				RedactionVersion: core.OutputRedactionVersion1,
				RedactionStatus:  core.OutputRedactionApplied,
			},
		}}))

		result := cmd.Execute()

		require.Equal(t, core.CommandError, result.Signal)
		var target *core.UnresolvedPathError
		require.ErrorAs(t, result.Err, &target)
		require.Equal(t, "parameters.absent", target.Path)
		_, err := os.Stat(marker)
		require.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("malformed source fails declaration parsing", func(t *testing.T) {
		_, err := catalog.ParseToolDefs([]byte(`
tools:
  - name: run_corpus_ingest
    type: exec
    binary: "true"
    parameters:
      type: object
      properties:
        directory:
          type: string
          source: $from(seed
`))
		require.ErrorContains(t, err, `tool "run_corpus_ingest" parameter "directory"`)
		require.ErrorContains(t, err, "must be a $from(label).path selector")
	})
}

func sourceTouchBuilder(marker, source string) *ExecBuilder {
	return &ExecBuilder{Def: catalog.ToolDef{
		Name:   "source_touch",
		Binary: "touch",
		Args:   []string{marker},
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"directory": map[string]interface{}{
					"type": "string", "flag": "--directory", "source": source,
				},
			},
			"required": []interface{}{"directory"},
		},
	}}
}

func findParamMapping(t *testing.T, mappings []catalog.ParamMapping, name string) catalog.ParamMapping {
	t.Helper()
	for _, mapping := range mappings {
		if mapping.Name == name {
			return mapping
		}
	}
	t.Fatalf("parameter mapping %q not found", name)
	return catalog.ParamMapping{}
}

// An array parameter's source, a JSON array in an earlier step's output,
// maps each element to its own flag occurrence (srd023 R2.12).
func TestExecBuilderMapsAnArraySource(t *testing.T) {
	td := catalog.ToolDef{
		Name:   "merge_branch",
		Binary: "echo",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"closes": map[string]interface{}{"type": "array", "flag": "--closes", "source": "$from(gate).closes"},
			},
		},
	}
	run := func(gate string) []string {
		cmd := (&ExecBuilder{Def: td, Root: t.TempDir()}).Build(core.Result{Output: `{}`}).(*ExecCmd)
		cmd.SetCommandState(core.NewCommandStateView(core.Execution{{
			CommandName: "merge_gate",
			Label:       "gate",
			Result: core.ResultDigest{Output: gate, RedactionVersion: core.OutputRedactionVersion1,
				RedactionStatus: core.OutputRedactionApplied},
		}}))
		require.NoError(t, cmd.resolveSourceParams())
		return cmd.buildArgs()
	}
	assert.Equal(t, []string{"--closes", "g/p#3", "--closes", "g/p#4"}, run(`{"closes":["g/p#3","g/p#4"]}`))
	assert.Empty(t, run(`{"closes":[]}`))
}
