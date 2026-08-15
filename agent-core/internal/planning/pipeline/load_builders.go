// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package pipeline

import (
	"fmt"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/extract"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/graph"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/spec"
)

// SigGraphLoaded signals that the requirement graph is built and seeded into
// pipeline state, so the machine can proceed to task extraction.
const SigGraphLoaded core.Signal = "GraphLoaded"

// load_graph is the pipeline's entry action: it loads the specification corpus
// from the run directory, builds the requirement dependency graph, and seeds it
// (with a fresh extractor) into pipeline state. Before this word existed the
// planner machine jumped straight to extract_task/select_all_ready against a nil
// Graph and panicked; load_graph is the missing declared word that populates
// State.Graph so extraction has something to traverse.

// LoadGraphBuilder constructs load_graph commands.
type LoadGraphBuilder struct {
	PS *State
}

func (b *LoadGraphBuilder) Build(_ core.Result) core.Command {
	return &loadGraphCmd{ps: b.PS}
}

func (b *LoadGraphBuilder) BuildReverser() core.Command {
	return &loadGraphCmd{ps: b.PS}
}

type loadGraphCmd struct {
	ps *State
}

func (c *loadGraphCmd) Name() string { return "load_graph" }

func (c *loadGraphCmd) Undo(prior core.Result) core.Result {
	return undoPipelineReceipt(c.Name(), c.ps, nil, prior.Receipt)
}

func (c *loadGraphCmd) Execute() (result core.Result) {
	snapshot := snapshotPipelineState(c.ps)
	defer func() { result = withPipelineReceipt(result, snapshot, nil) }()

	corpus, err := spec.LoadCorpus(c.ps.Directory)
	if err != nil {
		return core.Result{Signal: core.CommandError, Err: err, CommandName: c.Name(),
			Output: fmt.Sprintf("load_graph: load corpus from %q: %v", c.ps.Directory, err)}
	}
	g, err := graph.BuildGraph(corpus)
	if err != nil {
		return core.Result{Signal: core.CommandError, Err: err, CommandName: c.Name(),
			Output: fmt.Sprintf("load_graph: build graph: %v", err)}
	}

	c.ps.Corpus = corpus
	c.ps.Graph = g
	if c.ps.Extractor == nil {
		c.ps.Extractor = extract.NewExtractor()
	}
	if c.ps.Tracer != nil {
		c.ps.Tracer.Event("pipeline.graph_loaded",
			attribute.Int("graph.node_count", g.NodeCount()),
			attribute.Int("corpus.srd_count", len(corpus.SRDs)),
		)
	}
	return core.Result{
		CommandName: c.Name(),
		Signal:      SigGraphLoaded,
		Output:      fmt.Sprintf("loaded requirement graph: %d nodes from %d SRDs", g.NodeCount(), len(corpus.SRDs)),
	}
}
