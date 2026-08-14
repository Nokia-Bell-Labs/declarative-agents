// Copyright (c) 2026 Nokia. All rights reserved.

package validation

import (
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/spec"
)

// LoadTestClaimsBuilder loads only formal test suites for the evidence audit.
type LoadTestClaimsBuilder struct {
	ToolName          string
	VS                *SpecState
	ReferenceProvider DomainReferenceProvider
	SnapshotResolver  DomainSnapshotResolver
}

func (b *LoadTestClaimsBuilder) Build(_ core.Result) core.Command {
	return &loadTestClaimsCmd{
		toolName: b.ToolName,
		vs:       b.VS,
		undo:     newSpecUndoSupport(b.ReferenceProvider, b.SnapshotResolver),
	}
}

func (b *LoadTestClaimsBuilder) BuildReverser() core.Command {
	return &loadTestClaimsCmd{
		toolName: b.ToolName,
		vs:       b.VS,
		undo:     newSpecUndoSupport(nil, b.SnapshotResolver),
	}
}

type loadTestClaimsCmd struct {
	toolName string
	vs       *SpecState
	undo     specUndoSupport
}

func (c *loadTestClaimsCmd) Name() string {
	return validationCommandName(c.toolName, "load_test_claims")
}
func (c *loadTestClaimsCmd) Undo(prior core.Result) core.Result {
	return c.undo.restore(c.Name(), c.vs, prior)
}
func (c *loadTestClaimsCmd) Execute() core.Result {
	suites, err := spec.LoadTestSuites(c.vs.Directory)
	if err != nil {
		return evidenceReductionError(c.Name(), err)
	}
	if err := c.undo.capture(c.vs); err != nil {
		return specCaptureError(c.Name(), err)
	}
	c.vs.Corpus = &spec.Corpus{RootDir: c.vs.Directory, TestSuites: suites}
	return core.Result{
		Signal: core.ToolDone, CommandName: c.Name(),
		Output:  fmt.Sprintf("loaded %d formal test suites", len(suites)),
		Receipt: c.undo.receipt,
	}
}

// ResolveTestEvidenceBuilder reduces the three declared inventory commands into
// formal go_test resolution findings.
type ResolveTestEvidenceBuilder struct {
	ToolName          string
	VS                *SpecState
	ModuleFrom        string
	PackagesFrom      string
	TestsFrom         string
	ReferenceProvider DomainReferenceProvider
	SnapshotResolver  DomainSnapshotResolver
}

func (b *ResolveTestEvidenceBuilder) Build(_ core.Result) core.Command {
	return &resolveTestEvidenceCmd{
		toolName: b.ToolName,
		vs:       b.VS, moduleFrom: b.ModuleFrom,
		packagesFrom: b.PackagesFrom, testsFrom: b.TestsFrom,
		undo: newSpecUndoSupport(b.ReferenceProvider, b.SnapshotResolver),
	}
}

func (b *ResolveTestEvidenceBuilder) BuildReverser() core.Command {
	return &resolveTestEvidenceCmd{
		toolName: b.ToolName,
		vs:       b.VS, moduleFrom: b.ModuleFrom,
		packagesFrom: b.PackagesFrom, testsFrom: b.TestsFrom,
		undo: newSpecUndoSupport(nil, b.SnapshotResolver),
	}
}

type resolveTestEvidenceCmd struct {
	toolName                 string
	vs                       *SpecState
	moduleFrom, packagesFrom string
	testsFrom                string
	view                     core.CommandStateView
	undo                     specUndoSupport
}

func (c *resolveTestEvidenceCmd) Name() string {
	return validationCommandName(c.toolName, "resolve_test_evidence")
}
func (c *resolveTestEvidenceCmd) SetCommandState(view core.CommandStateView) {
	c.view = view
}
func (c *resolveTestEvidenceCmd) Undo(prior core.Result) core.Result {
	return c.undo.restore(c.Name(), c.vs, prior)
}

func (c *resolveTestEvidenceCmd) Execute() core.Result {
	module, err := resolveExecOutput(c.view, c.moduleFrom)
	if err != nil {
		return evidenceReductionError(c.Name(), err)
	}
	packages, err := resolveExecOutput(c.view, c.packagesFrom)
	if err != nil {
		return evidenceReductionError(c.Name(), err)
	}
	tests, err := resolveExecOutput(c.view, c.testsFrom)
	if err != nil {
		return evidenceReductionError(c.Name(), err)
	}
	inventory, err := spec.ParseGoTestInventory(module, packages, tests)
	if err != nil {
		return evidenceReductionError(c.Name(), err)
	}

	findings := spec.ValidateGoTestEvidence(inventory, c.vs.Corpus.TestSuites)
	if err := c.undo.capture(c.vs); err != nil {
		return specCaptureError(c.Name(), err)
	}
	c.vs.TestInventory = inventory
	c.vs.Findings = append(c.vs.Findings, findings...)
	res := evidenceValidationResult(c.Name(), c.vs)
	res.Receipt = c.undo.receipt
	return res
}

// ReduceTestEvidenceRunBuilder reduces one declared full-module test run.
type ReduceTestEvidenceRunBuilder struct {
	ToolName          string
	VS                *SpecState
	RunFrom           string
	ReferenceProvider DomainReferenceProvider
	SnapshotResolver  DomainSnapshotResolver
}

func (b *ReduceTestEvidenceRunBuilder) Build(_ core.Result) core.Command {
	return &reduceTestEvidenceRunCmd{
		toolName: b.ToolName,
		vs:       b.VS, runFrom: b.RunFrom,
		undo: newSpecUndoSupport(b.ReferenceProvider, b.SnapshotResolver),
	}
}

func (b *ReduceTestEvidenceRunBuilder) BuildReverser() core.Command {
	return &reduceTestEvidenceRunCmd{
		toolName: b.ToolName,
		vs:       b.VS, runFrom: b.RunFrom,
		undo: newSpecUndoSupport(nil, b.SnapshotResolver),
	}
}

type reduceTestEvidenceRunCmd struct {
	toolName string
	vs       *SpecState
	runFrom  string
	view     core.CommandStateView
	undo     specUndoSupport
}

func (c *reduceTestEvidenceRunCmd) Name() string {
	return validationCommandName(c.toolName, "reduce_test_evidence_run")
}
func (c *reduceTestEvidenceRunCmd) SetCommandState(view core.CommandStateView) {
	c.view = view
}
func (c *reduceTestEvidenceRunCmd) Undo(prior core.Result) core.Result {
	return c.undo.restore(c.Name(), c.vs, prior)
}

func (c *reduceTestEvidenceRunCmd) Execute() core.Result {
	if c.vs.TestInventory == nil {
		return evidenceReductionError(c.Name(), fmt.Errorf("test inventory was not resolved"))
	}
	output, err := resolveExecOutput(c.view, c.runFrom)
	if err != nil {
		return evidenceReductionError(c.Name(), err)
	}
	findings, err := spec.ReduceGoTestEvidenceRun(
		c.vs.TestInventory, c.vs.Corpus.TestSuites, output)
	if err != nil {
		return evidenceReductionError(c.Name(), err)
	}
	if err := c.undo.capture(c.vs); err != nil {
		return specCaptureError(c.Name(), err)
	}
	c.vs.Findings = append(c.vs.Findings, findings...)
	res := evidenceValidationResult(c.Name(), c.vs)
	res.Receipt = c.undo.receipt
	return res
}

func resolveExecOutput(view core.CommandStateView, selector string) (string, error) {
	value, err := core.ResolveFromSelector(view, selector)
	if err != nil {
		return "", fmt.Errorf("%s: %w", selector, err)
	}
	raw, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s resolved to %T, want string", selector, value)
	}
	return raw, nil
}

func evidenceValidationResult(name string, vs *SpecState) core.Result {
	spec.SortFindings(vs.Findings)
	errs := spec.Errors(vs.Findings)
	vs.HasErrors = len(errs) > 0
	res := validateSpecsResult(name, len(vs.Findings), len(errs))
	return res
}

func evidenceReductionError(name string, err error) core.Result {
	return core.Result{
		Signal: core.CommandError, CommandName: name,
		Output: fmt.Sprintf("%s failed: %v", name, err), Err: err,
	}
}

var (
	_ core.CommandStateAware = (*resolveTestEvidenceCmd)(nil)
	_ core.CommandStateAware = (*reduceTestEvidenceRunCmd)(nil)
	_ core.Reverser          = (*LoadTestClaimsBuilder)(nil)
	_ core.Reverser          = (*ResolveTestEvidenceBuilder)(nil)
	_ core.Reverser          = (*ReduceTestEvidenceRunBuilder)(nil)
)
