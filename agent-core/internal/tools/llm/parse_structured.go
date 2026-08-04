// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// CompileStructuredSchema compiles a declared JSON Schema during registration.
func CompileStructuredSchema(toolName string, declaration map[string]interface{}) (*jsonschema.Schema, error) {
	compiler := jsonschema.NewCompiler()
	location := "urn:declarative-agents:" + toolName
	if err := compiler.AddResource(location, declaration); err != nil {
		return nil, fmt.Errorf("tool %q schema: %w", toolName, err)
	}
	schema, err := compiler.Compile(location)
	if err != nil {
		return nil, fmt.Errorf("tool %q schema: %w", toolName, err)
	}
	return schema, nil
}

// ParseStructuredBuilder parses and validates selected prior model output.
type ParseStructuredBuilder struct {
	ToolName string
	Source   string
	Schema   *jsonschema.Schema
	Parsed   core.Signal
	Unparsed core.Signal
}

func (b ParseStructuredBuilder) Build(previous core.Result) core.Command {
	return &parseStructuredCmd{
		name: b.ToolName, source: b.Source, schema: b.Schema,
		parsed: b.Parsed, unparsed: b.Unparsed, previous: previous.Output,
	}
}

type parseStructuredCmd struct {
	name, source     string
	schema           *jsonschema.Schema
	parsed, unparsed core.Signal
	previous         string
	view             core.CommandStateView
}

func (c *parseStructuredCmd) Name() string                            { return c.name }
func (c *parseStructuredCmd) SetCommandState(v core.CommandStateView) { c.view = v }
func (c *parseStructuredCmd) Undo(_ core.Result) core.Result          { return core.NoopUndo(c.Name()) }

var _ core.CommandStateAware = (*parseStructuredCmd)(nil)

func (c *parseStructuredCmd) Execute() core.Result {
	var value interface{}
	if c.source == "$.output" {
		value = c.previous
	} else {
		resolved, err := core.ResolveFromSelector(c.view, c.source)
		if err != nil {
			return c.fault(err)
		}
		value = resolved
	}
	raw, ok := value.(string)
	if !ok {
		return c.unparsedResult(fmt.Sprintf("selected output is %T, want string", value))
	}
	var parsed interface{}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return c.unparsedResult(fmt.Sprintf("malformed JSON: %v", err))
	}
	if err := c.schema.Validate(parsed); err != nil {
		return c.unparsedResult(fmt.Sprintf("schema validation failed: %v", err))
	}
	output, err := json.Marshal(parsed)
	if err != nil {
		return c.fault(fmt.Errorf("encode parsed output: %w", err))
	}
	return core.Result{Signal: c.parsed, CommandName: c.Name(), Output: string(output)}
}

func (c *parseStructuredCmd) unparsedResult(message string) core.Result {
	return core.Result{Signal: c.unparsed, CommandName: c.Name(), Output: message}
}

func (c *parseStructuredCmd) fault(err error) core.Result {
	wrapped := fmt.Errorf("%s: %w", c.Name(), err)
	return core.Result{Signal: core.CommandError, CommandName: c.Name(), Output: wrapped.Error(), Err: wrapped}
}
