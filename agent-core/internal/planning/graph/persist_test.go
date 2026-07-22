// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package graph

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSaveLoadGraph_RoundTrip(t *testing.T) {
	corpus := loadTestCorpus(t)
	original, err := BuildGraph(corpus)
	require.NoError(t, err)

	// Mutate some state to verify persistence
	n, _ := original.Node("srd001-auth-R1.1")
	require.NoError(t, n.MarkPlanning())
	require.NoError(t, n.MarkExecuting())
	require.NoError(t, n.MarkDone())

	path := filepath.Join(t.TempDir(), "graph-state.yaml")
	require.NoError(t, SaveGraph(original, path))

	restored, err := LoadGraph(path)
	require.NoError(t, err)

	assert.Equal(t, original.NodeCount(), restored.NodeCount())

	// Verify node state preserved
	restoredNode, ok := restored.Node("srd001-auth-R1.1")
	require.True(t, ok)
	assert.Equal(t, Done, restoredNode.Status)

	// Verify edges preserved
	origEdges := original.Edges()
	restoredEdges := restored.Edges()
	assert.Equal(t, len(origEdges), len(restoredEdges))
	for i := range origEdges {
		assert.Equal(t, origEdges[i], restoredEdges[i])
	}
}

func TestSaveLoadGraph_AllNodes(t *testing.T) {
	corpus := loadTestCorpus(t)
	original, err := BuildGraph(corpus)
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "graph-state.yaml")
	require.NoError(t, SaveGraph(original, path))

	restored, err := LoadGraph(path)
	require.NoError(t, err)

	origNodes := original.Nodes()
	restNodes := restored.Nodes()
	require.Equal(t, len(origNodes), len(restNodes))

	for i := range origNodes {
		assert.Equal(t, origNodes[i].ID, restNodes[i].ID)
		assert.Equal(t, origNodes[i].SRDID, restNodes[i].SRDID)
		assert.Equal(t, origNodes[i].Group, restNodes[i].Group)
		assert.Equal(t, origNodes[i].Weight, restNodes[i].Weight)
		assert.Equal(t, origNodes[i].Status, restNodes[i].Status)
	}
}

func TestSaveLoadGraph_QueriesWorkAfterRestore(t *testing.T) {
	corpus := loadTestCorpus(t)
	g, err := BuildGraph(corpus)
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "graph-state.yaml")
	require.NoError(t, SaveGraph(g, path))

	restored, err := LoadGraph(path)
	require.NoError(t, err)

	// Verify Ready() works on restored graph
	ready := restored.Ready()
	assert.NotEmpty(t, ready)

	// Verify TopologicalSort works
	order, err := restored.TopologicalSort()
	require.NoError(t, err)
	assert.Len(t, order, restored.NodeCount())

	// Verify Predecessors works
	preds, err := restored.Predecessors("srd001-auth-R1.2")
	require.NoError(t, err)
	assert.Len(t, preds, 1)
}

func TestLoadGraph_FileNotFound(t *testing.T) {
	_, err := LoadGraph("nonexistent.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read graph state")
}

func TestSaveGraph_FailedRetries(t *testing.T) {
	corpus := loadTestCorpus(t)
	g, err := BuildGraph(corpus)
	require.NoError(t, err)

	n, _ := g.Node("srd001-auth-R1.1")
	require.NoError(t, n.MarkPlanning())
	require.NoError(t, n.MarkExecuting())
	require.NoError(t, n.MarkFailed())
	assert.Equal(t, 1, n.Retries)

	path := filepath.Join(t.TempDir(), "graph.yaml")
	require.NoError(t, SaveGraph(g, path))

	restored, err := LoadGraph(path)
	require.NoError(t, err)

	rn, ok := restored.Node("srd001-auth-R1.1")
	require.True(t, ok)
	assert.Equal(t, Failed, rn.Status)
	assert.Equal(t, 1, rn.Retries)
}

func TestLoadGraphRejectsInvalidPersistedState(t *testing.T) {
	t.Parallel()
	base := func() GraphState {
		return GraphState{
			Nodes: []Node{
				{ID: "A", SRDID: "srd-a", Group: "R1", Status: Pending},
				{ID: "B", SRDID: "srd-b", Group: "R1", Status: Done},
			},
			Edges: []Edge{{Source: "A", Target: "B"}},
		}
	}
	tests := []struct {
		name    string
		mutate  func(*GraphState)
		wantErr string
	}{
		{name: "unknown status", mutate: func(s *GraphState) { s.Nodes[0].Status = "wedged" }, wantErr: `nodes[0] "A" status "wedged" is invalid`},
		{name: "negative retries", mutate: func(s *GraphState) { s.Nodes[0].Retries = -1 }, wantErr: `nodes[0] "A" retries must be nonnegative`},
		{name: "failed without retry", mutate: func(s *GraphState) { s.Nodes[0].Status = Failed }, wantErr: `failed status requires at least one retry`},
		{name: "missing node id", mutate: func(s *GraphState) { s.Nodes[0].ID = "" }, wantErr: `nodes[0].id is required`},
		{name: "missing SRD id", mutate: func(s *GraphState) { s.Nodes[0].SRDID = "" }, wantErr: `srd_id is required`},
		{name: "duplicate node id", mutate: func(s *GraphState) { s.Nodes[1].ID = "A" }, wantErr: `nodes[1].id "A" duplicates nodes[0].id`},
		{name: "unknown edge source", mutate: func(s *GraphState) { s.Edges[0].Source = "missing" }, wantErr: `edges[0].source "missing" does not reference a node`},
		{name: "empty edge target", mutate: func(s *GraphState) { s.Edges[0].Target = "" }, wantErr: `edges[0].target is required`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			state := base()
			tt.mutate(&state)
			data, err := yaml.Marshal(state)
			require.NoError(t, err)
			path := filepath.Join(t.TempDir(), "graph.yaml")
			require.NoError(t, os.WriteFile(path, data, 0o600))
			_, err = LoadGraph(path)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestValidateGraphStateAcceptsReachableLifecycleStates(t *testing.T) {
	t.Parallel()
	tests := []Node{
		{ID: "pending", SRDID: "srd", Status: Pending},
		{ID: "planning", SRDID: "srd", Status: Planning},
		{ID: "executing", SRDID: "srd", Status: Executing},
		{ID: "done", SRDID: "srd", Status: Done},
		{ID: "failed", SRDID: "srd", Status: Failed, Retries: 1},
		{ID: "retried-pending", SRDID: "srd", Status: Pending, Retries: 2},
	}
	for _, node := range tests {
		t.Run(node.ID, func(t *testing.T) {
			t.Parallel()
			require.NoError(t, validateGraphState(GraphState{Nodes: []Node{node}}))
		})
	}
}
