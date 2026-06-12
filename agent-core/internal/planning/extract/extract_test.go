// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package extract

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/planning/graph"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/spec"
)

func buildTestGraph(t *testing.T) *graph.Graph {
	t.Helper()
	corpus, err := spec.LoadCorpus(
		filepath.Join("..", "..", "..", "pkg", "spec", "testdata", "valid"))
	require.NoError(t, err)
	g, err := graph.BuildGraph(corpus)
	require.NoError(t, err)
	return g
}

func TestExtractNext_FirstTask(t *testing.T) {
	g := buildTestGraph(t)
	ext := NewExtractor()

	task := ext.ExtractNext(g, 5)
	require.NotNil(t, task)

	assert.Equal(t, "srd001-auth", task.SRDID)
	assert.NotEmpty(t, task.NodeIDs)
	assert.True(t, task.Weight <= 5, "task weight %d exceeds threshold 5", task.Weight)
}

func TestExtractNext_RespectWeightThreshold(t *testing.T) {
	g := buildTestGraph(t)
	ext := NewExtractor()

	task := ext.ExtractNext(g, 2)
	require.NotNil(t, task)

	assert.True(t, task.Weight <= 2 || len(task.NodeIDs) == 1,
		"task weight %d exceeds threshold 2 with %d nodes", task.Weight, len(task.NodeIDs))
}

func TestExtractNext_OversizedNodeSoloTask(t *testing.T) {
	g := buildTestGraph(t)
	ext := NewExtractor()

	// srd001-auth-R1.2 has weight 2; with threshold 1, it should be a solo task
	task := ext.ExtractNext(g, 1)
	require.NotNil(t, task)
	assert.Equal(t, 1, len(task.NodeIDs))

	markTaskDone(t, g, task)

	task = ext.ExtractNext(g, 1)
	require.NotNil(t, task)
	assert.Equal(t, 1, len(task.NodeIDs))
	assert.Equal(t, "srd001-auth-R1.2", task.NodeIDs[0])
	assert.Equal(t, 2, task.Weight)
}

func TestExtractNext_UnlimitedWeight(t *testing.T) {
	g := buildTestGraph(t)
	ext := NewExtractor()

	task := ext.ExtractNext(g, 0)
	require.NotNil(t, task)
	assert.NotEmpty(t, task.NodeIDs)
}

func TestExtractNext_DefaultWeight(t *testing.T) {
	g := buildTestGraph(t)
	ext := NewExtractor()

	task := ext.ExtractNext(g, -1)
	require.NotNil(t, task)
	assert.True(t, task.Weight <= 5 || len(task.NodeIDs) == 1)
}

func TestExtractNext_SingleSRDOnly(t *testing.T) {
	g := buildTestGraph(t)
	ext := NewExtractor()

	task := ext.ExtractNext(g, 100)
	require.NotNil(t, task)
	assert.Equal(t, "srd001-auth", task.SRDID)

	for _, id := range task.NodeIDs {
		n, ok := g.Node(id)
		require.True(t, ok)
		assert.Equal(t, task.SRDID, n.SRDID, "node %s should belong to %s", id, task.SRDID)
	}
}

func TestExtractNext_ReturnsNilWhenNoReady(t *testing.T) {
	g := buildTestGraph(t)
	ext := NewExtractor()

	n, _ := g.Node("srd001-auth-R1.1")
	require.NoError(t, n.MarkPlanning())
	require.NoError(t, n.MarkExecuting())
	require.NoError(t, n.MarkFailed())

	task := ext.ExtractNext(g, 5)
	assert.Nil(t, task)
}

func TestExtractNext_DFSAffinity(t *testing.T) {
	g := buildTestGraph(t)
	ext := NewExtractor()

	task1 := ext.ExtractNext(g, 5)
	require.NotNil(t, task1)
	assert.Equal(t, "srd001-auth", task1.SRDID)
	assert.Equal(t, "srd001-auth", ext.HintSRD())

	markTaskDone(t, g, task1)

	task2 := ext.ExtractNext(g, 5)
	require.NotNil(t, task2)
	assert.Equal(t, "srd001-auth", task2.SRDID, "should prefer hint SRD")
}

func TestExtractNext_TaskID(t *testing.T) {
	g := buildTestGraph(t)
	ext := NewExtractor()

	task := ext.ExtractNext(g, 1)
	require.NotNil(t, task)

	if len(task.NodeIDs) == 1 {
		assert.Equal(t, task.NodeIDs[0], task.ID)
	} else {
		expected := task.NodeIDs[0] + ".." + task.NodeIDs[len(task.NodeIDs)-1]
		assert.Equal(t, expected, task.ID)
	}
}

func TestExtractNext_ExhaustAllTasks(t *testing.T) {
	g := buildTestGraph(t)
	ext := NewExtractor()

	var tasks []*Task
	for {
		task := ext.ExtractNext(g, 100)
		if task == nil {
			break
		}
		tasks = append(tasks, task)
		markTaskDone(t, g, task)
	}

	assert.NotEmpty(t, tasks)

	var allNodeIDs []string
	for _, task := range tasks {
		allNodeIDs = append(allNodeIDs, task.NodeIDs...)
	}
	assert.Equal(t, 8, len(allNodeIDs), "should cover all 8 nodes")
}

func markTaskDone(t *testing.T, g *graph.Graph, task *Task) {
	t.Helper()
	for _, id := range task.NodeIDs {
		n, ok := g.Node(id)
		require.True(t, ok)
		require.NoError(t, n.MarkPlanning())
		require.NoError(t, n.MarkExecuting())
		require.NoError(t, n.MarkDone())
	}
}
