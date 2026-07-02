// Copyright (c) 2026 Nokia. All rights reserved.

package validation

import (
	"fmt"
	"io"
	"os"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/spec"
)

// SpecState holds shared state across spec validation tools.
type SpecState struct {
	Directory string
	Stderr    io.Writer
	Corpus    *spec.Corpus
	Graph     *spec.Graph
	Findings  []spec.Finding
	HasErrors bool
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
func (c *loadCorpusCmd) Undo() core.Result {
	return undoSpecSnapshot(c.Name(), c.vs, c.snapshot, c.hasSnapshot)
}
func (c *loadCorpusCmd) UndoMemento() (core.UndoMemento, error) {
	return specMemento(c.Name(), c.snapshot, c.hasSnapshot)
}

func (c *loadCorpusCmd) Execute() core.Result {
	corpus, err := spec.LoadCorpus(c.vs.Directory)
	if err != nil {
		return core.Result{Signal: core.CommandError, Err: err, Output: fmt.Sprintf("load corpus failed: %v", err), CommandName: c.Name()}
	}
	c.snapshot = snapshotSpec(c.vs)
	c.hasSnapshot = true
	c.vs.Corpus = corpus
	output := fmt.Sprintf("loaded %d SRDs, %d use cases, %d test suites, %d machines, %d tool declarations",
		len(corpus.SRDs), len(corpus.UseCases), len(corpus.TestSuites), len(corpus.Machines), len(corpus.ToolDeclarations))
	return core.Result{Signal: core.ToolDone, Output: output, CommandName: c.Name()}
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
func (c *validateSpecsCmd) Undo() core.Result {
	return undoSpecSnapshot(c.Name(), c.vs, c.snapshot, c.hasSnapshot)
}
func (c *validateSpecsCmd) UndoMemento() (core.UndoMemento, error) {
	return specMemento(c.Name(), c.snapshot, c.hasSnapshot)
}

func (c *validateSpecsCmd) Execute() core.Result {
	c.snapshot = snapshotSpec(c.vs)
	c.hasSnapshot = true
	g, err := spec.BuildGraph(c.vs.Corpus)
	if err != nil {
		return core.Result{Signal: core.CommandError, Err: err, Output: fmt.Sprintf("build graph failed: %v", err), CommandName: c.Name()}
	}
	c.vs.Graph = g
	c.vs.Findings = spec.Validate(g, c.vs.Corpus)
	errs := spec.Errors(c.vs.Findings)
	c.vs.HasErrors = len(errs) > 0
	return validateSpecsResult(c.Name(), len(c.vs.Findings), len(errs))
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
	VS *SpecState
}

func (b *FormatReportBuilder) Build(_ core.Result) core.Command {
	return &formatReportCmd{vs: b.VS}
}

type formatReportCmd struct {
	vs *SpecState
}

func (c *formatReportCmd) Name() string      { return "format_report" }
func (c *formatReportCmd) Undo() core.Result { return core.NoopUndo(c.Name()) }

func (c *formatReportCmd) Execute() core.Result {
	report := spec.FormatFindings(c.vs.Findings)
	summary := specSummary(c.vs)
	if c.vs.HasErrors {
		output := fmt.Sprintf("%s\nvalidate: %s — %d error(s)", report, summary, len(spec.Errors(c.vs.Findings)))
		fmt.Fprintln(c.vs.stderr(), output)
		return core.Result{Signal: core.ToolFailed, Output: output, CommandName: c.Name()}
	}
	output := fmt.Sprintf("%s\nvalidate: %s — OK", report, summary)
	fmt.Fprintln(c.vs.stderr(), output)
	return core.Result{Signal: core.ToolDone, Output: output, CommandName: c.Name()}
}

func specSummary(vs *SpecState) string {
	return fmt.Sprintf("%d SRDs, %d use cases, %d test suites, %d machines, %d tool declarations, %d nodes, %d edges",
		len(vs.Corpus.SRDs), len(vs.Corpus.UseCases), len(vs.Corpus.TestSuites),
		len(vs.Corpus.Machines), len(vs.Corpus.ToolDeclarations),
		vs.Graph.NodeCount(), len(vs.Graph.Edges()))
}
