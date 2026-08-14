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
	ToolName          string
	VS                *SpecState
	ReferenceProvider DomainReferenceProvider
	SnapshotResolver  DomainSnapshotResolver
}

func (b *LoadCorpusBuilder) Build(_ core.Result) core.Command {
	return &loadCorpusCmd{
		toolName: b.ToolName,
		vs:       b.VS,
		undo:     newSpecUndoSupport(b.ReferenceProvider, b.SnapshotResolver),
	}
}

func (b *LoadCorpusBuilder) BuildReverser() core.Command {
	return &loadCorpusCmd{
		toolName: b.ToolName,
		vs:       b.VS,
		undo:     newSpecUndoSupport(nil, b.SnapshotResolver),
	}
}

type loadCorpusCmd struct {
	toolName string
	vs       *SpecState
	undo     specUndoSupport
}

func (c *loadCorpusCmd) Name() string {
	return validationCommandName(c.toolName, "load_corpus")
}
func (c *loadCorpusCmd) Undo(prior core.Result) core.Result {
	return c.undo.restore(c.Name(), c.vs, prior)
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
	output, err := loadedCorpusOutput(c.vs.Directory, corpus, charters)
	if err != nil {
		return core.Result{Signal: core.CommandError, Err: err, Output: err.Error(), CommandName: c.Name()}
	}
	if err := c.undo.capture(c.vs); err != nil {
		return specCaptureError(c.Name(), err)
	}
	c.vs.TargetDirectory = c.vs.Directory
	c.vs.Corpus = corpus
	c.vs.Charters = charters
	return core.Result{Signal: core.ToolDone, Output: output, CommandName: c.Name(), Receipt: c.undo.receipt}
}

func loadedCorpusOutput(targetDirectory string, corpus *spec.Corpus, charters []spec.Charter) (string, error) {
	summary := fmt.Sprintf("loaded %d SRDs, %d use cases, %d test suites, %d machines, %d tool declarations",
		len(corpus.SRDs), len(corpus.UseCases), len(corpus.TestSuites), len(corpus.Machines), len(corpus.ToolDeclarations))
	if len(charters) > 0 {
		summary = fmt.Sprintf("%s, %d charters", summary, len(charters))
	}
	grepChecks, err := spec.BuildGrepSearchPlans(targetDirectory, charters)
	if err != nil {
		return "", fmt.Errorf("prepare grep checks failed: %w", err)
	}
	refChecks, err := spec.BuildRefSearchPlans(targetDirectory, charters)
	if err != nil {
		return "", fmt.Errorf("prepare ref checks failed: %w", err)
	}
	consistencyChecks, err := spec.BuildConsistencyScanPlans(targetDirectory, charters)
	if err != nil {
		return "", fmt.Errorf("prepare consistency checks failed: %w", err)
	}
	output, err := json.Marshal(map[string]interface{}{
		"summary": summary, "grep_checks": grepChecks, "ref_checks": refChecks,
		"consistency_checks": consistencyChecks,
	})
	if err != nil {
		return "", fmt.Errorf("encode loaded corpus: %w", err)
	}
	return string(output), nil
}

// ValidateSpecsBuilder builds the graph and runs consistency checks.
type ValidateSpecsBuilder struct {
	ToolName          string
	VS                *SpecState
	ReferenceProvider DomainReferenceProvider
	SnapshotResolver  DomainSnapshotResolver
}

func (b *ValidateSpecsBuilder) Build(_ core.Result) core.Command {
	return &validateSpecsCmd{
		toolName: b.ToolName,
		vs:       b.VS,
		undo:     newSpecUndoSupport(b.ReferenceProvider, b.SnapshotResolver),
	}
}

func (b *ValidateSpecsBuilder) BuildReverser() core.Command {
	return &validateSpecsCmd{
		toolName: b.ToolName,
		vs:       b.VS,
		undo:     newSpecUndoSupport(nil, b.SnapshotResolver),
	}
}

type validateSpecsCmd struct {
	toolName string
	vs       *SpecState
	undo     specUndoSupport
}

func (c *validateSpecsCmd) Name() string {
	return validationCommandName(c.toolName, "validate_specs")
}
func (c *validateSpecsCmd) Undo(prior core.Result) core.Result {
	return c.undo.restore(c.Name(), c.vs, prior)
}

func (c *validateSpecsCmd) Execute() core.Result {
	g, err := spec.BuildGraph(c.vs.Corpus)
	if err != nil {
		return core.Result{Signal: core.CommandError, Err: err, Output: fmt.Sprintf("build graph failed: %v", err), CommandName: c.Name()}
	}
	findings, err := spec.ExecuteCharters(c.vs.TargetDirectory, g, c.vs.Corpus, c.vs.Charters)
	if err != nil {
		return core.Result{Signal: core.CommandError, Err: err, Output: fmt.Sprintf("execute charters failed: %v", err), CommandName: c.Name()}
	}
	if err := c.undo.capture(c.vs); err != nil {
		return specCaptureError(c.Name(), err)
	}
	c.vs.Graph = g
	c.vs.Findings = findings
	errs := spec.Errors(c.vs.Findings)
	c.vs.HasErrors = len(errs) > 0
	res := validateSpecsResult(c.Name(), len(c.vs.Findings), len(errs))
	res.Receipt = c.undo.receipt
	return res
}

func validateSpecsResult(commandName string, findings, errs int) core.Result {
	output := fmt.Sprintf("found %d findings (%d errors)", findings, errs)
	if errs > 0 {
		return core.Result{Signal: core.ValidationFailed, Output: output, CommandName: commandName}
	}
	return core.Result{Signal: core.ValidationPassed, Output: output, CommandName: commandName}
}

// FormatReportBuilder formats and outputs the findings report.
type FormatReportBuilder struct {
	ToolName string
	VS       *SpecState
}

func (b *FormatReportBuilder) Build(_ core.Result) core.Command {
	return &formatReportCmd{toolName: b.ToolName, vs: b.VS}
}

type formatReportCmd struct {
	toolName string
	vs       *SpecState
}

func (c *formatReportCmd) Name() string {
	return validationCommandName(c.toolName, "format_report")
}
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

func specCaptureError(commandName string, err error) core.Result {
	err = fmt.Errorf("%s: capture validation undo state: %w", commandName, err)
	return core.Result{
		Signal: core.CommandError, CommandName: commandName,
		Output: err.Error(), Err: err,
	}
}

var (
	_ core.Reverser = (*LoadCorpusBuilder)(nil)
	_ core.Reverser = (*ValidateSpecsBuilder)(nil)
)
