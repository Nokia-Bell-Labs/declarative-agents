// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/extract"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// shippedValidCorpus is the on-disk corpus used across the planning packages;
// exercising load_graph against it proves the word works on a real profile
// specification, not only synthesized fixtures.
func shippedValidCorpus() string {
	return filepath.Join("..", "..", "..", "pkg", "spec", "testdata", "valid")
}

const validLoadSRD = `id: srd-ok
title: OK
problem: test
requirements:
  R1:
    title: Stuff
    items:
      - R1.1: Do something.
`

const invalidLoadSRD = `id: srd-bad
title: Bad
problem: test
requirements:
  R1:
    title: Stuff
    items:
      - R1.1: Do something.
depends_on:
  - srd_id: nonexistent-srd
    symbols_used: [Foo]
`

func writeCorpusScaffold(t *testing.T, root string) {
	t.Helper()
	for _, d := range []string{
		filepath.Join("docs", "specs", "software-requirements"),
		filepath.Join("docs", "specs", "use-cases"),
		filepath.Join("docs", "specs", "test-suites"),
	} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, d), 0o755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "road-map.yaml"),
		[]byte("id: test\ntitle: Test\nreleases: []\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "SPECIFICATIONS.yaml"),
		[]byte("id: test\ntitle: Test\n"), 0o644))
}

func writeCorpusSRD(t *testing.T, root, filename, data string) {
	t.Helper()
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "docs", "specs", "software-requirements", filename),
		[]byte(data), 0o644))
}

// loadableState is a pipeline State primed for load_graph: no graph, corpus, or
// extractor yet, so the command's seeding and its rollback are observable.
func loadableState(t *testing.T, dir string, tracer tracing.Tracer) *State {
	t.Helper()
	return &State{
		Directory: dir,
		MaxWeight: 10,
		Tracer:    tracer,
		Ctx:       context.Background(),
	}
}

func TestLoadGraphBuilder_LoadsCorpusAndSeedsState(t *testing.T) {
	t.Parallel()

	tempValid := t.TempDir()
	writeCorpusScaffold(t, tempValid)
	writeCorpusSRD(t, tempValid, "srd001-ok.yaml", validLoadSRD)

	cases := []struct {
		name string
		dir  string
	}{
		{name: "shipped valid corpus", dir: shippedValidCorpus()},
		{name: "synthesized valid corpus", dir: tempValid},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tracer := tracing.NewRecordingTracer()
			ps := loadableState(t, tc.dir, tracer)

			result := (&LoadGraphBuilder{PS: ps}).Build(core.Result{}).Execute()

			require.Equal(t, SigGraphLoaded, result.Signal, result.Output)
			require.Contains(t, result.Output, "loaded requirement graph")
			require.NotEmpty(t, result.Receipt)
			require.NotNil(t, ps.Graph, "graph must be seeded")
			require.NotNil(t, ps.Corpus, "corpus must be seeded")
			require.NotNil(t, ps.Extractor, "extractor must be seeded")
			require.Greater(t, ps.Graph.NodeCount(), 0)
			require.NotEmpty(t, ps.Corpus.SRDs)

			event := tracer.FindEvent("pipeline.graph_loaded")
			require.NotNil(t, event, "graph_loaded event must be recorded")
			require.Equal(t, int64(ps.Graph.NodeCount()), event.Attrs["graph.node_count"])
			require.Equal(t, int64(len(ps.Corpus.SRDs)), event.Attrs["corpus.srd_count"])
		})
	}
}

func TestLoadGraphBuilder_FailureStatesPreserveState(t *testing.T) {
	t.Parallel()

	invalidDir := t.TempDir()
	writeCorpusScaffold(t, invalidDir)
	writeCorpusSRD(t, invalidDir, "srd001-bad.yaml", invalidLoadSRD)

	cases := []struct {
		name       string
		dir        string
		wantErrSub string
	}{
		{name: "missing docs directory", dir: t.TempDir(), wantErrSub: "load_graph: load corpus"},
		{name: "unresolvable dependency", dir: invalidDir, wantErrSub: "nonexistent-srd"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tracer := tracing.NewRecordingTracer()
			ps := loadableState(t, tc.dir, tracer)

			result := (&LoadGraphBuilder{PS: ps}).Build(core.Result{}).Execute()

			require.Equal(t, core.CommandError, result.Signal)
			require.Error(t, result.Err)
			require.Contains(t, result.Output, "load_graph: load corpus")
			require.Contains(t, result.Output, tc.wantErrSub)
			require.Nil(t, ps.Graph, "failed load must not seed a graph")
			require.Nil(t, ps.Corpus, "failed load must not seed a corpus")
			require.Nil(t, ps.Extractor, "failed load must not seed an extractor")
			require.Nil(t, tracer.FindEvent("pipeline.graph_loaded"),
				"failed load must not record the loaded event")
			require.NotEmpty(t, result.Receipt, "even failures carry a rollback receipt")
		})
	}
}

func TestLoadGraphCmd_UndoRestoresPreLoadState(t *testing.T) {
	t.Parallel()

	ps := loadableState(t, shippedValidCorpus(), tracing.NewRecordingTracer())
	builder := &LoadGraphBuilder{PS: ps}

	result := builder.Build(core.Result{}).Execute()
	require.Equal(t, SigGraphLoaded, result.Signal, result.Output)
	require.NotNil(t, ps.Graph)
	require.NotNil(t, ps.Corpus)
	require.NotNil(t, ps.Extractor)

	undo := builder.BuildReverser().Undo(result)
	require.Equal(t, core.ToolDone, undo.Signal, undo.Output)
	require.Nil(t, ps.Graph, "undo must clear the loaded graph")
	require.Nil(t, ps.Corpus, "undo must clear the loaded corpus")
	require.Nil(t, ps.Extractor, "undo must clear the seeded extractor")
}

func TestLoadGraphCmd_UndoKeepsPreexistingExtractor(t *testing.T) {
	t.Parallel()

	ps := loadableState(t, shippedValidCorpus(), tracing.NewRecordingTracer())
	existing := extract.NewExtractor()
	ps.Extractor = existing
	builder := &LoadGraphBuilder{PS: ps}

	result := builder.Build(core.Result{}).Execute()
	require.Equal(t, SigGraphLoaded, result.Signal, result.Output)
	require.Same(t, existing, ps.Extractor, "load_graph must not replace an existing extractor")

	undo := builder.BuildReverser().Undo(result)
	require.Equal(t, core.ToolDone, undo.Signal, undo.Output)
	require.Same(t, existing, ps.Extractor, "undo must keep a pre-existing extractor")
	require.Nil(t, ps.Graph, "undo still clears the graph it loaded")
}

func TestLoadGraphCmd_UndoRequiresReceipt(t *testing.T) {
	t.Parallel()

	ps := loadableState(t, t.TempDir(), tracing.NewRecordingTracer())
	undo := (&LoadGraphBuilder{PS: ps}).BuildReverser().Undo(core.Result{})

	require.Equal(t, core.CommandError, undo.Signal)
	require.Contains(t, undo.Output, "pipeline receipt is required")
}
