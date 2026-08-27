// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package compose

import (
	"encoding/json"
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// NormalizeVectorBuilder passes a flat vector through or unwraps one nested row.
type NormalizeVectorBuilder struct {
	ToolName string
	Path     string
	Signal   core.Signal
}

// ValidateNormalizeVectorConfig checks the local map-key path and success signal.
func ValidateNormalizeVectorConfig(toolName, path, signal string) error {
	parsed, ok := core.ParseSelector("$." + path)
	if !ok || parsed.Label != "" {
		return fmt.Errorf("tool %q: config path %q is not a dotted map-key path", toolName, path)
	}
	if signal == "" {
		return fmt.Errorf("tool %q: config requires signal", toolName)
	}
	return nil
}

func (b NormalizeVectorBuilder) Build(previous core.Result) core.Command {
	return &normalizeVectorCmd{
		name: b.ToolName, path: b.Path, signal: b.Signal,
		previous: previous.Output, redaction: previous.Redaction,
	}
}

type normalizeVectorCmd struct {
	name, path, previous string
	signal               core.Signal
	redaction            core.OutputRedaction
}

func (c *normalizeVectorCmd) Name() string                   { return c.name }
func (c *normalizeVectorCmd) Undo(_ core.Result) core.Result { return core.NoopUndo(c.Name()) }

func (c *normalizeVectorCmd) Execute() core.Result {
	var decoded interface{}
	if err := json.Unmarshal([]byte(c.previous), &decoded); err != nil {
		return transformFault(c.Name(), fmt.Errorf("decode previous result object: %w", err))
	}
	envelope, ok := decoded.(map[string]interface{})
	if !ok {
		return transformFault(c.Name(), fmt.Errorf("previous result is not an object"))
	}
	container, key, value, err := vectorField(envelope, c.path)
	if err != nil {
		return transformFault(c.Name(), err)
	}
	vector, err := normalizeVector(value)
	if err != nil {
		return transformFault(c.Name(), fmt.Errorf("field %q: %w", c.path, err))
	}
	container[key] = vector
	data, err := json.Marshal(envelope)
	if err != nil {
		return transformFault(c.Name(), fmt.Errorf("encode normalized result: %w", err))
	}
	return core.Result{
		Signal: c.signal, CommandName: c.Name(), Output: string(data),
		Redaction: c.redaction,
	}
}

func vectorField(
	envelope map[string]interface{},
	path string,
) (map[string]interface{}, string, interface{}, error) {
	parsed, ok := core.ParseSelector("$." + path)
	if !ok || parsed.Label != "" {
		return nil, "", nil, fmt.Errorf("path %q is not a dotted map-key path", path)
	}
	container := envelope
	for index, component := range parsed.Path {
		value, exists := container[component]
		if !exists {
			return nil, "", nil, fmt.Errorf("field %q is absent", path)
		}
		if index == len(parsed.Path)-1 {
			return container, component, value, nil
		}
		next, ok := value.(map[string]interface{})
		if !ok {
			return nil, "", nil, fmt.Errorf(
				"field %q component %q resolved to %T, want object",
				path, component, value,
			)
		}
		container = next
	}
	return nil, "", nil, fmt.Errorf("field %q is empty", path)
}

func normalizeVector(value interface{}) ([]interface{}, error) {
	outer, ok := value.([]interface{})
	if !ok {
		return nil, fmt.Errorf("resolved to %T, want numeric vector or single-row matrix", value)
	}
	if len(outer) == 0 {
		return nil, fmt.Errorf("vector is empty")
	}
	if row, nested := outer[0].([]interface{}); nested {
		if len(outer) != 1 {
			return nil, fmt.Errorf("matrix has %d rows, want exactly one", len(outer))
		}
		if len(row) == 0 {
			return nil, fmt.Errorf("matrix row is empty")
		}
		if err := requireNumericVector(row); err != nil {
			return nil, err
		}
		return row, nil
	}
	if err := requireNumericVector(outer); err != nil {
		return nil, err
	}
	return outer, nil
}

func requireNumericVector(vector []interface{}) error {
	for index, value := range vector {
		if _, ok := value.(float64); !ok {
			return fmt.Errorf("vector[%d] resolved to %T, want number", index, value)
		}
	}
	return nil
}
