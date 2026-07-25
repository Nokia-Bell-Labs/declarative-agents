// Copyright (c) 2026 Nokia. All rights reserved.

package validation

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/spec"
)

// SpecState holds shared state across spec validation tools.
type SpecState struct {
	Directory       string
	TargetDirectory string
	SuitePaths      []string
	Stderr          io.Writer
	Corpus          *spec.Corpus
	Graph           *spec.Graph
	Charters        []spec.Charter
	Findings        []spec.Finding
	TestInventory   *spec.GoTestInventory
	HasErrors       bool
	CorpusOptional  bool
}

func (vs *SpecState) stderr() io.Writer {
	if vs.Stderr != nil {
		return vs.Stderr
	}
	return os.Stderr
}

// LoadCorpusBuilder loads spec artifacts from the project directory.
type LoadCorpusBuilder struct {
	VS *SpecState
}

func (b *LoadCorpusBuilder) Build(_ core.Result) core.Command {
	return &loadCorpusCmd{vs: b.VS}
}

type loadCorpusCmd struct {
	vs          *SpecState
	snapshot    specSnapshot
	hasSnapshot bool
}

func (c *loadCorpusCmd) Name() string { return "load_corpus" }
func (c *loadCorpusCmd) Undo(prior core.Result) core.Result {
	return undoSpecState(c.Name(), c.vs, prior, c.snapshot, c.hasSnapshot)
}

func (c *loadCorpusCmd) Execute() core.Result {
	var opts []spec.CorpusOption
	if c.vs.CorpusOptional {
		opts = append(opts, spec.WithOptionalCorpus())
	}
	corpus, err := spec.LoadCorpus(c.vs.Directory, opts...)
	if err != nil {
		return core.Result{Signal: core.CommandError, Err: err, Output: fmt.Sprintf("load corpus failed: %v", err), CommandName: c.Name()}
	}
	charters, err := spec.LoadCharters(c.vs.SuitePaths)
	if err != nil {
		return core.Result{Signal: core.CommandError, Err: err, Output: fmt.Sprintf("load charters failed: %v", err), CommandName: c.Name()}
	}
	c.snapshot = snapshotSpec(c.vs)
	c.hasSnapshot = true
	c.vs.TargetDirectory = c.vs.Directory
	c.vs.Corpus = corpus
	c.vs.Charters = charters
	output, err := loadedCorpusOutput(c.vs, corpus, charters)
	if err != nil {
		return core.Result{Signal: core.CommandError, Err: err, Output: err.Error(), CommandName: c.Name()}
	}
	return core.Result{Signal: core.ToolDone, Output: output, CommandName: c.Name(), Receipt: encodeSpecReceipt(c.snapshot)}
}

func loadedCorpusOutput(vs *SpecState, corpus *spec.Corpus, charters []spec.Charter) (string, error) {
	summary := fmt.Sprintf("loaded %d SRDs, %d use cases, %d test suites, %d machines, %d tool declarations",
		len(corpus.SRDs), len(corpus.UseCases), len(corpus.TestSuites), len(corpus.Machines), len(corpus.ToolDeclarations))
	if len(charters) > 0 {
		summary = fmt.Sprintf("%s, %d charters", summary, len(charters))
	}
	grepChecks, err := spec.BuildGrepSearchPlans(vs.TargetDirectory, charters)
	if err != nil {
		return "", fmt.Errorf("prepare grep checks failed: %w", err)
	}
	output, err := json.Marshal(map[string]interface{}{"summary": summary, "grep_checks": grepChecks})
	if err != nil {
		return "", fmt.Errorf("encode loaded corpus: %w", err)
	}
	return string(output), nil
}

// ValidateSpecsBuilder builds the graph and runs consistency checks.
type ValidateSpecsBuilder struct {
	VS *SpecState
}

func (b *ValidateSpecsBuilder) Build(_ core.Result) core.Command {
	return &validateSpecsCmd{vs: b.VS}
}

type validateSpecsCmd struct {
	vs          *SpecState
	snapshot    specSnapshot
	hasSnapshot bool
}

func (c *validateSpecsCmd) Name() string { return "validate_specs" }
func (c *validateSpecsCmd) Undo(prior core.Result) core.Result {
	return undoSpecState(c.Name(), c.vs, prior, c.snapshot, c.hasSnapshot)
}

func (c *validateSpecsCmd) Execute() core.Result {
	c.snapshot = snapshotSpec(c.vs)
	c.hasSnapshot = true
	g, err := spec.BuildGraph(c.vs.Corpus)
	if err != nil {
		return core.Result{Signal: core.CommandError, Err: err, Output: fmt.Sprintf("build graph failed: %v", err), CommandName: c.Name()}
	}
	c.vs.Graph = g
	findings, err := spec.ExecuteCharters(c.vs.TargetDirectory, g, c.vs.Corpus, c.vs.Charters)
	if err != nil {
		return core.Result{Signal: core.CommandError, Err: err, Output: fmt.Sprintf("execute charters failed: %v", err), CommandName: c.Name()}
	}
	c.vs.Findings = findings
	errs := spec.Errors(c.vs.Findings)
	c.vs.HasErrors = len(errs) > 0
	res := validateSpecsResult(c.Name(), len(c.vs.Findings), len(errs))
	res.Receipt = encodeSpecReceipt(c.snapshot)
	return res
}

func validateSpecsResult(commandName string, findings, errs int) core.Result {
	output := fmt.Sprintf("found %d findings (%d errors)", findings, errs)
	if errs > 0 {
		return core.Result{Signal: core.ValidationFailed, Output: output, CommandName: commandName}
	}
	return core.Result{Signal: core.ValidationPassed, Output: output, CommandName: commandName}
}

// ReduceGrepChecksBuilder shapes joined rg outcomes into validation findings.
type ReduceGrepChecksBuilder struct {
	VS          *SpecState
	ResultsFrom string
}

func (b *ReduceGrepChecksBuilder) Build(_ core.Result) core.Command {
	return &reduceGrepChecksCmd{vs: b.VS, resultsFrom: b.ResultsFrom}
}

type reduceGrepChecksCmd struct {
	vs          *SpecState
	resultsFrom string
	view        core.CommandStateView
	snapshot    specSnapshot
	hasSnapshot bool
}

type joinedGrepOutcome struct {
	Input  spec.GrepSearchPlan `json:"input"`
	Result struct {
		Output string `json:"output"`
	} `json:"result"`
}

func (c *reduceGrepChecksCmd) Name() string { return "reduce_grep_checks" }
func (c *reduceGrepChecksCmd) SetCommandState(view core.CommandStateView) {
	c.view = view
}
func (c *reduceGrepChecksCmd) Undo(prior core.Result) core.Result {
	return undoSpecState(c.Name(), c.vs, prior, c.snapshot, c.hasSnapshot)
}

func (c *reduceGrepChecksCmd) Execute() core.Result {
	outcomes, err := resolveGrepOutcomes(c.view, c.resultsFrom)
	if err != nil {
		return grepReductionError(c.Name(), err)
	}
	c.snapshot = snapshotSpec(c.vs)
	c.hasSnapshot = true
	for _, outcome := range outcomes {
		findings, err := reduceGrepOutcome(outcome)
		if err != nil {
			return grepReductionError(c.Name(), err)
		}
		c.vs.Findings = append(c.vs.Findings, findings...)
	}
	spec.SortFindings(c.vs.Findings)
	errs := spec.Errors(c.vs.Findings)
	c.vs.HasErrors = len(errs) > 0
	res := validateSpecsResult(c.Name(), len(c.vs.Findings), len(errs))
	res.Receipt = encodeSpecReceipt(c.snapshot)
	return res
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
	return core.Result{
		Signal: core.CommandError, CommandName: commandName,
		Output: fmt.Sprintf("reduce grep checks failed: %v", err), Err: err,
	}
}

var _ core.CommandStateAware = (*reduceGrepChecksCmd)(nil)

// FormatReportBuilder formats and outputs the findings report.
type FormatReportBuilder struct {
	VS *SpecState
}

func (b *FormatReportBuilder) Build(_ core.Result) core.Command {
	return &formatReportCmd{vs: b.VS}
}

type formatReportCmd struct {
	vs *SpecState
}

func (c *formatReportCmd) Name() string                   { return "format_report" }
func (c *formatReportCmd) Undo(_ core.Result) core.Result { return core.NoopUndo(c.Name()) }

func (c *formatReportCmd) Execute() core.Result {
	report := spec.FormatFindings(c.vs.Findings)
	summary := specSummary(c.vs)
	if c.vs.HasErrors {
		output := fmt.Sprintf("%s\nvalidate: %s — %d error(s)", report, summary, len(spec.Errors(c.vs.Findings)))
		_, _ = fmt.Fprintln(c.vs.stderr(), output)
		return core.Result{Signal: core.ToolFailed, Output: output, CommandName: c.Name()}
	}
	output := fmt.Sprintf("%s\nvalidate: %s — OK", report, summary)
	_, _ = fmt.Fprintln(c.vs.stderr(), output)
	return core.Result{Signal: core.ToolDone, Output: output, CommandName: c.Name()}
}

func specSummary(vs *SpecState) string {
	nodes, edges := 0, 0
	if vs.Graph != nil {
		nodes, edges = vs.Graph.NodeCount(), len(vs.Graph.Edges())
	}
	return fmt.Sprintf("%d SRDs, %d use cases, %d test suites, %d machines, %d tool declarations, %d nodes, %d edges",
		len(vs.Corpus.SRDs), len(vs.Corpus.UseCases), len(vs.Corpus.TestSuites),
		len(vs.Corpus.Machines), len(vs.Corpus.ToolDeclarations),
		nodes, edges)
}
