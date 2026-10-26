// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

// A parameter present with a type the tool cannot take is named as a type
// mismatch, not as missing, so the model fixes the call instead of resending
// it; an optional one fails the same way rather than being dropped.
func TestExecBuilder_WrongType(t *testing.T) {
	td := catalog.ToolDef{
		Name:   "write",
		Binary: "echo",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"items": map[string]interface{}{"type": "string", "flag": "--items"},
				"note":  map[string]interface{}{"type": "string", "flag": "--note"},
				"tags":  map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "flag": "--tag"},
			},
			"required": []interface{}{"items"},
		},
	}
	builder := &ExecBuilder{Def: td, Root: "/tmp"}
	for request, want := range map[string]string{
		`{"parameters":{"items":["a","b"]}}`:            "parameter items must be a string, got array",
		`{"parameters":{"items":"a","note":{"x":1}}}`:   "parameter note must be a string, got object",
		`{"parameters":{"items":"a","tags":{"x":"y"}}}`: "parameter tags must be an array of strings, got object",
	} {
		res := builder.Build(core.Result{Output: request}).Execute()
		assert.Equal(t, core.ToolFailed, res.Signal, request)
		assert.Equal(t, want, res.Output, request)
	}
	res := builder.Build(core.Result{Output: `{"parameters":{"items":"a","note":null}}`})
	assert.IsType(t, &ExecCmd{}, res, "a null optional parameter is absent")
}
