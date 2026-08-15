// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package validation

import (
	"encoding/json"
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/spec"
)

// ReduceGrepChecksBuilder shapes joined rg outcomes into validation findings.
type ReduceGrepChecksBuilder struct {
	ToolName          string
	VS                *SpecState
	ResultsFrom       string
	ReferenceProvider DomainReferenceProvider
	SnapshotResolver  DomainSnapshotResolver
}

func (b *ReduceGrepChecksBuilder) Build(_ core.Result) core.Command {
	return &reduceGrepChecksCmd{
		toolName: b.ToolName,
		vs:       b.VS, resultsFrom: b.ResultsFrom,
		undo: newSpecUndoSupport(b.ReferenceProvider, b.SnapshotResolver),
	}
}

func (b *ReduceGrepChecksBuilder) BuildReverser() core.Command {
	return &reduceGrepChecksCmd{
		toolName: b.ToolName,
		vs:       b.VS, resultsFrom: b.ResultsFrom,
		undo: newSpecUndoSupport(nil, b.SnapshotResolver),
	}
}

type reduceGrepChecksCmd struct {
	toolName    string
	vs          *SpecState
	resultsFrom string
	view        core.CommandStateView
	undo        specUndoSupport
}

type joinedGrepOutcome struct {
	Input  spec.GrepSearchPlan `json:"input"`
	Result struct {
		Output string `json:"output"`
	} `json:"result"`
}

func (c *reduceGrepChecksCmd) Name() string {
	return validationCommandName(c.toolName, "reduce_grep_checks")
}

func (c *reduceGrepChecksCmd) SetCommandState(view core.CommandStateView) { c.view = view }

func (c *reduceGrepChecksCmd) Undo(prior core.Result) core.Result {
	return c.undo.restore(c.Name(), c.vs, prior)
}

func (c *reduceGrepChecksCmd) Execute() core.Result {
	outcomes, err := resolveGrepOutcomes(c.view, c.resultsFrom)
	if err != nil {
		return grepReductionError(c.Name(), err)
	}
	var added []spec.Finding
	for _, outcome := range outcomes {
		findings, err := reduceGrepOutcome(outcome)
		if err != nil {
			return grepReductionError(c.Name(), err)
		}
		added = append(added, findings...)
	}
	return finishSpecReduction(c.Name(), c.vs, &c.undo, added)
}

func resolveGrepOutcomes(view core.CommandStateView, selector string) ([]joinedGrepOutcome, error) {
	value, err := core.ResolveFromSelector(view, selector)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode joined outcomes: %w", err)
	}
	var outcomes []joinedGrepOutcome
	if err := json.Unmarshal(data, &outcomes); err != nil {
		return nil, fmt.Errorf("decode joined outcomes: %w", err)
	}
	return outcomes, nil
}

func reduceGrepOutcome(outcome joinedGrepOutcome) ([]spec.Finding, error) {
	var search struct {
		Output   string `json:"output"`
		ExitCode int    `json:"exit_code"`
	}
	if err := json.Unmarshal([]byte(outcome.Result.Output), &search); err != nil {
		return nil, fmt.Errorf("charter %q check %q: decode structured rg result: %w",
			outcome.Input.SuiteID, outcome.Input.CheckID, err)
	}
	return spec.ReduceGrepSearch(outcome.Input, search.Output, search.ExitCode)
}

func grepReductionError(commandName string, err error) core.Result {
	return reductionError(commandName, "grep", err)
}

// ReduceRefChecksBuilder shapes joined external ref scans into findings.
type ReduceRefChecksBuilder struct {
	ToolName          string
	VS                *SpecState
	ResultsFrom       string
	ReferenceProvider DomainReferenceProvider
	SnapshotResolver  DomainSnapshotResolver
}

func (b *ReduceRefChecksBuilder) Build(_ core.Result) core.Command {
	return &reduceRefChecksCmd{
		toolName: b.ToolName,
		vs:       b.VS, resultsFrom: b.ResultsFrom,
		undo: newSpecUndoSupport(b.ReferenceProvider, b.SnapshotResolver),
	}
}

func (b *ReduceRefChecksBuilder) BuildReverser() core.Command {
	return &reduceRefChecksCmd{
		toolName: b.ToolName,
		vs:       b.VS, resultsFrom: b.ResultsFrom,
		undo: newSpecUndoSupport(nil, b.SnapshotResolver),
	}
}

type reduceRefChecksCmd struct {
	toolName    string
	vs          *SpecState
	resultsFrom string
	view        core.CommandStateView
	undo        specUndoSupport
}

type joinedRefOutcome struct {
	Input  spec.RefSearchPlan `json:"input"`
	Result struct {
		Output string `json:"output"`
	} `json:"result"`
}

func (c *reduceRefChecksCmd) Name() string {
	return validationCommandName(c.toolName, "reduce_ref_checks")
}

func (c *reduceRefChecksCmd) SetCommandState(view core.CommandStateView) { c.view = view }

func (c *reduceRefChecksCmd) Undo(prior core.Result) core.Result {
	return c.undo.restore(c.Name(), c.vs, prior)
}

func (c *reduceRefChecksCmd) Execute() core.Result {
	outcomes, err := resolveRefOutcomes(c.view, c.resultsFrom)
	if err != nil {
		return refReductionError(c.Name(), err)
	}
	added, err := reduceRefOutcomes(outcomes)
	if err != nil {
		return refReductionError(c.Name(), err)
	}
	return finishSpecReduction(c.Name(), c.vs, &c.undo, added)
}

func resolveRefOutcomes(view core.CommandStateView, selector string) ([]joinedRefOutcome, error) {
	value, err := core.ResolveFromSelector(view, selector)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode joined outcomes: %w", err)
	}
	var outcomes []joinedRefOutcome
	if err := json.Unmarshal(data, &outcomes); err != nil {
		return nil, fmt.Errorf("decode joined outcomes: %w", err)
	}
	return outcomes, nil
}

func reduceRefOutcomes(outcomes []joinedRefOutcome) ([]spec.Finding, error) {
	var added []spec.Finding
	for _, outcome := range outcomes {
		var scan struct {
			Output string `json:"output"`
		}
		if err := json.Unmarshal([]byte(outcome.Result.Output), &scan); err != nil {
			return nil, fmt.Errorf(
				"charter %q check %q: decode structured ref scan: %w",
				outcome.Input.SuiteID, outcome.Input.CheckID, err,
			)
		}
		findings, err := spec.ReduceRefSearch(outcome.Input, scan.Output)
		if err != nil {
			return nil, err
		}
		added = append(added, findings...)
	}
	return added, nil
}

func refReductionError(commandName string, err error) core.Result {
	return reductionError(commandName, "ref", err)
}

// ReduceConsistencyChecksBuilder reduces joined external file scans.
type ReduceConsistencyChecksBuilder struct {
	ToolName          string
	VS                *SpecState
	ResultsFrom       string
	ReferenceProvider DomainReferenceProvider
	SnapshotResolver  DomainSnapshotResolver
}

func (b *ReduceConsistencyChecksBuilder) Build(_ core.Result) core.Command {
	return &reduceConsistencyChecksCmd{
		toolName: b.ToolName,
		vs:       b.VS, resultsFrom: b.ResultsFrom,
		undo: newSpecUndoSupport(b.ReferenceProvider, b.SnapshotResolver),
	}
}

func (b *ReduceConsistencyChecksBuilder) BuildReverser() core.Command {
	return &reduceConsistencyChecksCmd{
		toolName: b.ToolName,
		vs:       b.VS, resultsFrom: b.ResultsFrom,
		undo: newSpecUndoSupport(nil, b.SnapshotResolver),
	}
}

type reduceConsistencyChecksCmd struct {
	toolName    string
	vs          *SpecState
	resultsFrom string
	view        core.CommandStateView
	undo        specUndoSupport
}

type joinedConsistencyOutcome struct {
	Input  spec.ConsistencyScanPlan `json:"input"`
	Result struct {
		Output string `json:"output"`
	} `json:"result"`
}

func (c *reduceConsistencyChecksCmd) Name() string {
	return validationCommandName(c.toolName, "reduce_consistency_checks")
}

func (c *reduceConsistencyChecksCmd) SetCommandState(view core.CommandStateView) { c.view = view }

func (c *reduceConsistencyChecksCmd) Undo(prior core.Result) core.Result {
	return c.undo.restore(c.Name(), c.vs, prior)
}

func (c *reduceConsistencyChecksCmd) Execute() core.Result {
	outcomes, err := resolveConsistencyOutcomes(c.view, c.resultsFrom)
	if err != nil {
		return consistencyReductionError(c.Name(), err)
	}
	added, err := reduceConsistencyOutcomes(outcomes)
	if err != nil {
		return consistencyReductionError(c.Name(), err)
	}
	return finishSpecReduction(c.Name(), c.vs, &c.undo, added)
}

func resolveConsistencyOutcomes(
	view core.CommandStateView,
	selector string,
) ([]joinedConsistencyOutcome, error) {
	value, err := core.ResolveFromSelector(view, selector)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode joined outcomes: %w", err)
	}
	var outcomes []joinedConsistencyOutcome
	if err := json.Unmarshal(data, &outcomes); err != nil {
		return nil, fmt.Errorf("decode joined outcomes: %w", err)
	}
	return outcomes, nil
}

func reduceConsistencyOutcomes(outcomes []joinedConsistencyOutcome) ([]spec.Finding, error) {
	var added []spec.Finding
	for _, outcome := range outcomes {
		var scan struct {
			Output string `json:"output"`
		}
		if err := json.Unmarshal([]byte(outcome.Result.Output), &scan); err != nil {
			return nil, fmt.Errorf(
				"charter %q check %q: decode structured consistency scan: %w",
				outcome.Input.SuiteID, outcome.Input.Check.ID, err,
			)
		}
		findings, err := spec.ReduceConsistencyScan(outcome.Input, scan.Output)
		if err != nil {
			return nil, err
		}
		added = append(added, findings...)
	}
	return added, nil
}

func consistencyReductionError(commandName string, err error) core.Result {
	return reductionError(commandName, "consistency", err)
}

func finishSpecReduction(
	commandName string,
	vs *SpecState,
	undo *specUndoSupport,
	added []spec.Finding,
) core.Result {
	if err := undo.capture(vs); err != nil {
		return specCaptureError(commandName, err)
	}
	vs.Findings = append(vs.Findings, added...)
	spec.SortFindings(vs.Findings)
	errs := spec.Errors(vs.Findings)
	vs.HasErrors = len(errs) > 0
	result := validateSpecsResult(commandName, len(vs.Findings), len(errs))
	result.Receipt = undo.receipt
	return result
}

func reductionError(commandName, family string, err error) core.Result {
	return core.Result{
		Signal: core.CommandError, CommandName: commandName,
		Output: fmt.Sprintf("reduce %s checks failed: %v", family, err), Err: err,
	}
}

var (
	_ core.CommandStateAware = (*reduceGrepChecksCmd)(nil)
	_ core.CommandStateAware = (*reduceRefChecksCmd)(nil)
	_ core.CommandStateAware = (*reduceConsistencyChecksCmd)(nil)
	_ core.Reverser          = (*ReduceGrepChecksBuilder)(nil)
	_ core.Reverser          = (*ReduceRefChecksBuilder)(nil)
	_ core.Reverser          = (*ReduceConsistencyChecksBuilder)(nil)
)
