// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"fmt"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/spec"
)

// ValidateSpecState holds shared state across the validate spec tools.
type ValidateSpecState struct {
	Directory string
	Corpus    *spec.Corpus
	Graph     *spec.Graph
	Findings  []spec.Finding
	HasErrors bool
}

// LoadCorpusBuilder loads spec artifacts from the project directory.
type LoadCorpusBuilder struct {
	VS *ValidateSpecState
}

func (b *LoadCorpusBuilder) Build(_ core.Result) core.Command {
	return &loadCorpusCmd{vs: b.VS}
}

type loadCorpusCmd struct {
	vs          *ValidateSpecState
	snapshot    validateSpecSnapshot
	hasSnapshot bool
}

func (c *loadCorpusCmd) Name() string { return "load_corpus" }
func (c *loadCorpusCmd) Undo() core.Result {
	return undoValidateSpecSnapshot(c.Name(), c.vs, c.snapshot, c.hasSnapshot)
}
func (c *loadCorpusCmd) UndoMemento() (core.UndoMemento, error) {
	return validateSpecMemento(c.Name(), c.snapshot, c.hasSnapshot)
}

func (c *loadCorpusCmd) Execute() core.Result {
	corpus, err := spec.LoadCorpus(c.vs.Directory)
	if err != nil {
		return core.Result{
			Signal:      core.CommandError,
			Err:         err,
			Output:      fmt.Sprintf("load corpus failed: %v", err),
			CommandName: "load_corpus",
		}
	}
	c.snapshot = snapshotValidateSpec(c.vs)
	c.hasSnapshot = true
	c.vs.Corpus = corpus
	return core.Result{
		Signal: core.ToolDone,
		Output: fmt.Sprintf("loaded %d SRDs, %d use cases, %d test suites, %d machines",
			len(corpus.SRDs), len(corpus.UseCases), len(corpus.TestSuites), len(corpus.Machines)),
		CommandName: "load_corpus",
	}
}

// ValidateSpecsBuilder builds the graph and runs consistency checks.
type ValidateSpecsBuilder struct {
	VS *ValidateSpecState
}

func (b *ValidateSpecsBuilder) Build(_ core.Result) core.Command {
	return &validateSpecsCmd{vs: b.VS}
}

type validateSpecsCmd struct {
	vs          *ValidateSpecState
	snapshot    validateSpecSnapshot
	hasSnapshot bool
}

func (c *validateSpecsCmd) Name() string { return "validate_specs" }
func (c *validateSpecsCmd) Undo() core.Result {
	return undoValidateSpecSnapshot(c.Name(), c.vs, c.snapshot, c.hasSnapshot)
}
func (c *validateSpecsCmd) UndoMemento() (core.UndoMemento, error) {
	return validateSpecMemento(c.Name(), c.snapshot, c.hasSnapshot)
}

func (c *validateSpecsCmd) Execute() core.Result {
	c.snapshot = snapshotValidateSpec(c.vs)
	c.hasSnapshot = true
	g, err := spec.BuildGraph(c.vs.Corpus)
	if err != nil {
		return core.Result{
			Signal:      core.CommandError,
			Err:         err,
			Output:      fmt.Sprintf("build graph failed: %v", err),
			CommandName: "validate_specs",
		}
	}
	c.vs.Graph = g

	findings := spec.Validate(g, c.vs.Corpus)
	c.vs.Findings = findings

	errs := spec.Errors(findings)
	c.vs.HasErrors = len(errs) > 0

	if c.vs.HasErrors {
		return core.Result{
			Signal:      core.ValidationFailed,
			Output:      fmt.Sprintf("found %d findings (%d errors)", len(findings), len(errs)),
			CommandName: "validate_specs",
		}
	}
	return core.Result{
		Signal:      core.ValidationPassed,
		Output:      fmt.Sprintf("found %d findings (0 errors)", len(findings)),
		CommandName: "validate_specs",
	}
}

// FormatReportBuilder formats and outputs the findings report.
type FormatReportBuilder struct {
	VS *ValidateSpecState
}

func (b *FormatReportBuilder) Build(_ core.Result) core.Command {
	return &formatReportCmd{vs: b.VS}
}

type formatReportCmd struct {
	vs *ValidateSpecState
}

func (c *formatReportCmd) Name() string      { return "format_report" }
func (c *formatReportCmd) Undo() core.Result { return core.NoopUndo(c.Name()) }

func (c *formatReportCmd) Execute() core.Result {
	report := spec.FormatFindings(c.vs.Findings)

	summary := fmt.Sprintf("%d SRDs, %d use cases, %d test suites, %d machines, %d nodes, %d edges",
		len(c.vs.Corpus.SRDs), len(c.vs.Corpus.UseCases), len(c.vs.Corpus.TestSuites),
		len(c.vs.Corpus.Machines),
		c.vs.Graph.NodeCount(), len(c.vs.Graph.Edges()))

	if c.vs.HasErrors {
		errs := spec.Errors(c.vs.Findings)
		return core.Result{
			Signal:      core.ToolFailed,
			Output:      fmt.Sprintf("%s\nvalidate: %s — %d error(s)", report, summary, len(errs)),
			CommandName: "format_report",
		}
	}
	return core.Result{
		Signal:      core.ToolDone,
		Output:      fmt.Sprintf("%s\nvalidate: %s — OK", report, summary),
		CommandName: "format_report",
	}
}
