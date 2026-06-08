// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package graph

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/spec"
)

func loadTestCorpus(t *testing.T) *spec.Corpus {
	t.Helper()
	c, err := spec.LoadCorpus(filepath.Join("..", "spec", "testdata", "valid"))
	require.NoError(t, err)
	return c
}

func TestBuildGraph_NodeCount(t *testing.T) {
	corpus := loadTestCorpus(t)
	g, err := BuildGraph(corpus)
	require.NoError(t, err)

	// srd001-auth: R1(3) + R2(1) = 4
	// srd002-api: R1(2) = 2
	// srd003-storage: R1(2) = 2
	assert.Equal(t, 8, g.NodeCount())
}

func TestBuildGraph_NodeMetadata(t *testing.T) {
	corpus := loadTestCorpus(t)
	g, err := BuildGraph(corpus)
	require.NoError(t, err)

	n, ok := g.Node("srd001-auth-R1.2")
	require.True(t, ok)
	assert.Equal(t, "srd001-auth", n.SRDID)
	assert.Equal(t, "R1", n.Group)
	assert.Equal(t, 2, n.Weight)
	assert.Equal(t, Pending, n.Status)
	assert.Equal(t, 0, n.Retries)
}

func TestBuildGraph_IntraGroupEdges(t *testing.T) {
	corpus := loadTestCorpus(t)
	g, err := BuildGraph(corpus)
	require.NoError(t, err)

	// R1.1 → R1.2 → R1.3 within srd001-auth
	preds, err := g.Predecessors("srd001-auth-R1.2")
	require.NoError(t, err)
	require.Len(t, preds, 1)
	assert.Equal(t, "srd001-auth-R1.1", preds[0].ID)

	preds, err = g.Predecessors("srd001-auth-R1.3")
	require.NoError(t, err)
	require.Len(t, preds, 1)
	assert.Equal(t, "srd001-auth-R1.2", preds[0].ID)
}

func TestBuildGraph_InterGroupEdges(t *testing.T) {
	corpus := loadTestCorpus(t)
	g, err := BuildGraph(corpus)
	require.NoError(t, err)

	// srd001-auth: R1.3 (last of R1) → R2.1 (first of R2)
	preds, err := g.Predecessors("srd001-auth-R2.1")
	require.NoError(t, err)
	require.Len(t, preds, 1)
	assert.Equal(t, "srd001-auth-R1.3", preds[0].ID)
}

func TestBuildGraph_InterSRDEdges(t *testing.T) {
	corpus := loadTestCorpus(t)
	g, err := BuildGraph(corpus)
	require.NoError(t, err)

	// srd002-api depends_on srd001-auth
	// last of srd001-auth is R2.1, first of srd002-api is R1.1
	preds, err := g.Predecessors("srd002-api-R1.1")
	require.NoError(t, err)

	predIDs := make([]string, len(preds))
	for i, p := range preds {
		predIDs[i] = p.ID
	}
	assert.Contains(t, predIDs, "srd001-auth-R2.1")
}

func TestBuildGraph_TopologicalSort(t *testing.T) {
	corpus := loadTestCorpus(t)
	g, err := BuildGraph(corpus)
	require.NoError(t, err)

	order, err := g.TopologicalSort()
	require.NoError(t, err)
	assert.Len(t, order, 8)

	// srd001-auth items must come before srd002-api items
	authIdx := indexOf(order, "srd001-auth-R1.1")
	apiIdx := indexOf(order, "srd002-api-R1.1")
	assert.True(t, authIdx < apiIdx, "auth items should precede api items")
}

func TestBuildGraph_Ready_InitialState(t *testing.T) {
	corpus := loadTestCorpus(t)
	g, err := BuildGraph(corpus)
	require.NoError(t, err)

	ready := g.Ready()
	// Only root nodes (no predecessors) should be ready initially
	assert.NotEmpty(t, ready)

	// srd001-auth-R1.1 should be ready (no predecessors)
	var readyIDs []string
	for _, n := range ready {
		readyIDs = append(readyIDs, n.ID)
	}
	assert.Contains(t, readyIDs, "srd001-auth-R1.1")
	// srd001-auth-R1.2 should NOT be ready (depends on R1.1)
	assert.NotContains(t, readyIDs, "srd001-auth-R1.2")
}

func TestBuildGraph_Ready_AfterDone(t *testing.T) {
	corpus := loadTestCorpus(t)
	g, err := BuildGraph(corpus)
	require.NoError(t, err)

	n, _ := g.Node("srd001-auth-R1.1")
	require.NoError(t, n.MarkPlanning())
	require.NoError(t, n.MarkExecuting())
	require.NoError(t, n.MarkDone())

	ready := g.Ready()
	var readyIDs []string
	for _, n := range ready {
		readyIDs = append(readyIDs, n.ID)
	}
	assert.Contains(t, readyIDs, "srd001-auth-R1.2")
}

func TestBuildGraph_Blocked(t *testing.T) {
	corpus := loadTestCorpus(t)
	g, err := BuildGraph(corpus)
	require.NoError(t, err)

	n, _ := g.Node("srd001-auth-R1.1")
	require.NoError(t, n.MarkPlanning())
	require.NoError(t, n.MarkExecuting())
	require.NoError(t, n.MarkFailed())

	blocked := g.Blocked()
	var blockedIDs []string
	for _, b := range blocked {
		blockedIDs = append(blockedIDs, b.ID)
	}
	assert.Contains(t, blockedIDs, "srd001-auth-R1.2")
}

func TestBuildGraph_DFSFrom(t *testing.T) {
	corpus := loadTestCorpus(t)
	g, err := BuildGraph(corpus)
	require.NoError(t, err)

	// Initially only root nodes are ready
	reachable := g.DFSFrom("srd001-auth-R1.1")
	assert.NotEmpty(t, reachable)
	assert.Equal(t, "srd001-auth-R1.1", reachable[0].ID)
}

func TestBuildGraph_Edges(t *testing.T) {
	corpus := loadTestCorpus(t)
	g, err := BuildGraph(corpus)
	require.NoError(t, err)

	edges := g.Edges()
	assert.NotEmpty(t, edges)

	// Verify sorted order
	for i := 1; i < len(edges); i++ {
		prev := edges[i-1]
		curr := edges[i]
		assert.True(t, prev[0] < curr[0] || (prev[0] == curr[0] && prev[1] <= curr[1]))
	}
}

// --- State transition tests ---

func TestNode_StateTransitions_HappyPath(t *testing.T) {
	n := &Node{ID: "test", Status: Pending}

	require.NoError(t, n.MarkPlanning())
	assert.Equal(t, Planning, n.Status)

	require.NoError(t, n.MarkExecuting())
	assert.Equal(t, Executing, n.Status)

	require.NoError(t, n.MarkDone())
	assert.Equal(t, Done, n.Status)
}

func TestNode_StateTransitions_FailAndRetry(t *testing.T) {
	n := &Node{ID: "test", Status: Pending}

	require.NoError(t, n.MarkPlanning())
	require.NoError(t, n.MarkExecuting())
	require.NoError(t, n.MarkFailed())
	assert.Equal(t, Failed, n.Status)
	assert.Equal(t, 1, n.Retries)

	require.NoError(t, n.Reset())
	assert.Equal(t, Pending, n.Status)
	assert.Equal(t, 1, n.Retries) // retries preserved
}

func TestNode_StateTransitions_Invalid(t *testing.T) {
	n := &Node{ID: "test", Status: Pending}

	assert.Error(t, n.MarkExecuting(), "pending → executing should be invalid")
	assert.Error(t, n.MarkDone(), "pending → done should be invalid")
	assert.Error(t, n.MarkFailed(), "pending → failed should be invalid")

	require.NoError(t, n.MarkPlanning())
	assert.Error(t, n.MarkDone(), "planning → done should be invalid")
}

func TestNode_Nodes_Sorted(t *testing.T) {
	corpus := loadTestCorpus(t)
	g, err := BuildGraph(corpus)
	require.NoError(t, err)

	nodes := g.Nodes()
	for i := 1; i < len(nodes); i++ {
		assert.True(t, nodes[i-1].ID < nodes[i].ID,
			"nodes should be sorted: %s vs %s", nodes[i-1].ID, nodes[i].ID)
	}
}

func TestBuildGraph_ReleaseEdges(t *testing.T) {
	corpus := loadTestCorpus(t)
	g, err := BuildGraph(corpus)
	require.NoError(t, err)

	// srd003-storage is release 00.1, srd001-auth and srd002-api are 00.0
	// Release edges should connect last nodes of 00.0 SRDs to first of 00.1 SRDs
	// srd003-storage also has depends_on srd001-auth, so that edge exists already
	// But srd002-api last → srd003-storage first should exist as a release edge
	preds, err := g.Predecessors("srd003-storage-R1.1")
	require.NoError(t, err)

	predIDs := make([]string, len(preds))
	for i, p := range preds {
		predIDs[i] = p.ID
	}
	// Should have inter-SRD dep from srd001-auth AND release edge from srd002-api
	assert.Contains(t, predIDs, "srd001-auth-R2.1",
		"inter-SRD dependency edge from srd001-auth")
	assert.Contains(t, predIDs, "srd002-api-R1.2",
		"release edge from srd002-api (last of 00.0) to srd003-storage (first of 00.1)")
}

func TestBuildGraph_NodeRelease(t *testing.T) {
	corpus := loadTestCorpus(t)
	g, err := BuildGraph(corpus)
	require.NoError(t, err)

	n, ok := g.Node("srd001-auth-R1.1")
	require.True(t, ok)
	assert.Equal(t, "00.0", n.Release)

	n, ok = g.Node("srd003-storage-R1.1")
	require.True(t, ok)
	assert.Equal(t, "00.1", n.Release)
}

func indexOf(slice []string, item string) int {
	for i, s := range slice {
		if s == item {
			return i
		}
	}
	return -1
}
