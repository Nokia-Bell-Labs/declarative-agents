// Copyright (c) 2026 Nokia. All rights reserved.

package control

import (
	"encoding/json"
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// SelectSubsetBuilder constrains candidate names to a declared vocabulary.
type SelectSubsetBuilder struct {
	ToolName   string
	Candidates string
	Vocabulary string
	MatchField string
	AllMatched core.Signal
	Partial    core.Signal
	Empty      core.Signal
}

func ValidateSelectSubsetConfig(toolName, candidates, vocabulary, matchField, allMatched, partial, empty string) error {
	for name, selector := range map[string]string{"candidates": candidates, "vocabulary": vocabulary} {
		if _, _, ok := core.ParseFromSelector(selector); !ok {
			return fmt.Errorf("tool %q: config %s must be a $from(label).path selector", toolName, name)
		}
	}
	if matchField == "" {
		return fmt.Errorf("tool %q: config requires match_field", toolName)
	}
	if parsed, ok := core.ParseSelector("$." + matchField); !ok || parsed.Label != "" {
		return fmt.Errorf("tool %q: config match_field %q is not a dotted path", toolName, matchField)
	}
	for name, signal := range map[string]string{"all_matched": allMatched, "partial": partial, "empty": empty} {
		if signal == "" {
			return fmt.Errorf("tool %q: config requires %s signal", toolName, name)
		}
	}
	return nil
}

func (b SelectSubsetBuilder) Build(_ core.Result) core.Command {
	return &selectSubsetCmd{
		name: b.ToolName, candidates: b.Candidates, vocabulary: b.Vocabulary,
		matchField: b.MatchField, allMatched: b.AllMatched, partial: b.Partial, empty: b.Empty,
	}
}

type selectSubsetCmd struct {
	name, candidates, vocabulary, matchField string
	allMatched, partial, empty               core.Signal
	view                                     core.CommandStateView
}

func (c *selectSubsetCmd) Name() string                            { return c.name }
func (c *selectSubsetCmd) SetCommandState(v core.CommandStateView) { c.view = v }
func (c *selectSubsetCmd) Undo(_ core.Result) core.Result          { return core.NoopUndo(c.Name()) }

var _ core.CommandStateAware = (*selectSubsetCmd)(nil)

func (c *selectSubsetCmd) Execute() core.Result {
	candidates, vocabulary, err := c.resolveArrays()
	if err != nil {
		return c.fault(err)
	}
	declared, err := declaredEntries(vocabulary, c.matchField)
	if err != nil {
		return c.fault(err)
	}
	matched, unmatched, selected, err := selectDeclared(candidates, declared)
	if err != nil {
		return c.fault(err)
	}
	signal := c.outcome(len(matched), len(unmatched))
	output, err := json.Marshal(map[string]interface{}{
		"matched": matched, "unmatched": unmatched,
		"selected":      selected,
		"matched_count": len(matched), "unmatched_count": len(unmatched),
	})
	if err != nil {
		return c.fault(fmt.Errorf("encode output: %w", err))
	}
	return core.Result{Signal: signal, CommandName: c.Name(), Output: string(output)}
}

func (c *selectSubsetCmd) resolveArrays() ([]interface{}, []interface{}, error) {
	candidateValue, err := core.ResolveFromSelector(c.view, c.candidates)
	if err != nil {
		return nil, nil, err
	}
	vocabularyValue, err := core.ResolveFromSelector(c.view, c.vocabulary)
	if err != nil {
		return nil, nil, err
	}
	candidates, ok := candidateValue.([]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("candidates selector %q resolved to %T, want array", c.candidates, candidateValue)
	}
	vocabulary, ok := vocabularyValue.([]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("vocabulary selector %q resolved to %T, want array", c.vocabulary, vocabularyValue)
	}
	return candidates, vocabulary, nil
}

func declaredEntries(vocabulary []interface{}, matchField string) (map[string]interface{}, error) {
	declared := make(map[string]interface{}, len(vocabulary))
	for i, value := range vocabulary {
		name, err := vocabularyName(value, matchField)
		if err != nil {
			return nil, fmt.Errorf("vocabulary[%d]: %w", i, err)
		}
		if _, exists := declared[name]; !exists {
			declared[name] = value
		}
	}
	return declared, nil
}

func selectDeclared(candidates []interface{}, declared map[string]interface{}) ([]string, []string, []interface{}, error) {
	matched := make([]string, 0, len(candidates))
	unmatched := make([]string, 0, len(candidates))
	selected := make([]interface{}, 0, len(candidates))
	for i, value := range candidates {
		name, ok := value.(string)
		if !ok {
			return nil, nil, nil, fmt.Errorf("candidates[%d] is %T, want string", i, value)
		}
		if trusted, exists := declared[name]; exists {
			matched = append(matched, name)
			selected = append(selected, trusted)
		} else {
			unmatched = append(unmatched, name)
		}
	}
	return matched, unmatched, selected, nil
}

func (c *selectSubsetCmd) outcome(matched, unmatched int) core.Signal {
	signal := c.partial
	switch {
	case matched == 0:
		signal = c.empty
	case unmatched == 0:
		signal = c.allMatched
	}
	return signal
}

func vocabularyName(value interface{}, matchField string) (string, error) {
	if name, ok := value.(string); ok {
		return name, nil
	}
	value, err := fieldValue(value, matchField)
	if err != nil {
		return "", err
	}
	name, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("match_field %q resolved to %T, want string", matchField, value)
	}
	return name, nil
}

func (c *selectSubsetCmd) fault(err error) core.Result {
	wrapped := fmt.Errorf("%s: %w", c.Name(), err)
	return core.Result{Signal: core.CommandError, CommandName: c.Name(), Output: wrapped.Error(), Err: wrapped}
}
