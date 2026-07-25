// Copyright (c) 2026 Nokia. All rights reserved.

package validation

import (
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/spec"
)

// LoadTestClaimsBuilder loads only formal test suites for the evidence audit.
type LoadTestClaimsBuilder struct{ VS *SpecState }

func (b *LoadTestClaimsBuilder) Build(_ core.Result) core.Command {
	return &loadTestClaimsCmd{vs: b.VS}
}

type loadTestClaimsCmd struct {
	vs          *SpecState
	snapshot    specSnapshot
	hasSnapshot bool
}

func (c *loadTestClaimsCmd) Name() string { return "load_test_claims" }
func (c *loadTestClaimsCmd) Undo(prior core.Result) core.Result {
	return undoSpecState(c.Name(), c.vs, prior, c.snapshot, c.hasSnapshot)
}
func (c *loadTestClaimsCmd) Execute() core.Result {
	suites, err := spec.LoadTestSuites(c.vs.Directory)
	if err != nil {
		return evidenceReductionError(c.Name(), err)
	}
	c.snapshot = snapshotSpec(c.vs)
	c.hasSnapshot = true
	c.vs.Corpus = &spec.Corpus{RootDir: c.vs.Directory, TestSuites: suites}
	return core.Result{
		Signal: core.ToolDone, CommandName: c.Name(),
		Output:  fmt.Sprintf("loaded %d formal test suites", len(suites)),
		Receipt: encodeSpecReceipt(c.snapshot),
	}
}

// ResolveTestEvidenceBuilder reduces the three declared inventory commands into
// formal go_test resolution findings.
type ResolveTestEvidenceBuilder struct {
	VS           *SpecState
	ModuleFrom   string
	PackagesFrom string
	TestsFrom    string
}

func (b *ResolveTestEvidenceBuilder) Build(_ core.Result) core.Command {
	return &resolveTestEvidenceCmd{
		vs: b.VS, moduleFrom: b.ModuleFrom,
		packagesFrom: b.PackagesFrom, testsFrom: b.TestsFrom,
	}
}

type resolveTestEvidenceCmd struct {
	vs                       *SpecState
	moduleFrom, packagesFrom string
	testsFrom                string
	view                     core.CommandStateView
	snapshot                 specSnapshot
	hasSnapshot              bool
}

func (c *resolveTestEvidenceCmd) Name() string { return "resolve_test_evidence" }
func (c *resolveTestEvidenceCmd) SetCommandState(view core.CommandStateView) {
	c.view = view
}
func (c *resolveTestEvidenceCmd) Undo(prior core.Result) core.Result {
	return undoSpecState(c.Name(), c.vs, prior, c.snapshot, c.hasSnapshot)
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

	c.snapshot = snapshotSpec(c.vs)
	c.hasSnapshot = true
	c.vs.TestInventory = inventory
	c.vs.Findings = append(c.vs.Findings,
		spec.ValidateGoTestEvidence(inventory, c.vs.Corpus.TestSuites)...)
	res := evidenceValidationResult(c.Name(), c.vs)
	res.Receipt = encodeSpecReceipt(c.snapshot)
	return res
}

// ReduceTestEvidenceRunBuilder reduces one declared full-module test run.
type ReduceTestEvidenceRunBuilder struct {
	VS      *SpecState
	RunFrom string
}

func (b *ReduceTestEvidenceRunBuilder) Build(_ core.Result) core.Command {
	return &reduceTestEvidenceRunCmd{vs: b.VS, runFrom: b.RunFrom}
}

type reduceTestEvidenceRunCmd struct {
	vs          *SpecState
	runFrom     string
	view        core.CommandStateView
	snapshot    specSnapshot
	hasSnapshot bool
}

func (c *reduceTestEvidenceRunCmd) Name() string { return "reduce_test_evidence_run" }
func (c *reduceTestEvidenceRunCmd) SetCommandState(view core.CommandStateView) {
	c.view = view
}
func (c *reduceTestEvidenceRunCmd) Undo(prior core.Result) core.Result {
	return undoSpecState(c.Name(), c.vs, prior, c.snapshot, c.hasSnapshot)
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
	c.snapshot = snapshotSpec(c.vs)
	c.hasSnapshot = true
	c.vs.Findings = append(c.vs.Findings, findings...)
	res := evidenceValidationResult(c.Name(), c.vs)
	res.Receipt = encodeSpecReceipt(c.snapshot)
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
)
