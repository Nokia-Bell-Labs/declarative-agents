// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package graph

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/spec"
)

func loadTestCorpus(t *testing.T) *spec.Corpus {
	t.Helper()
	c, err := spec.LoadCorpus(filepath.Join("..", "..", "..", "pkg", "spec", "testdata", "valid"))
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
