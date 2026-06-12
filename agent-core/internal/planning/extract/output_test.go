// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package extract

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/planning/graph"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/spec"
)

func extractAllTasks(t *testing.T, g *graph.Graph) []*Task {
	t.Helper()
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
	return tasks
}

func TestBuildOutputTasks(t *testing.T) {
	corpus, err := spec.LoadCorpus(
		filepath.Join("..", "..", "..", "pkg", "spec", "testdata", "valid"))
	require.NoError(t, err)

	g, err := graph.BuildGraph(corpus)
	require.NoError(t, err)

	tasks := extractAllTasks(t, g)
	require.NotEmpty(t, tasks)

	output := BuildOutputTasks(tasks, g, corpus)
	assert.Len(t, output, len(tasks))

	for _, ot := range output {
		assert.NotEmpty(t, ot.ID)
		assert.NotEmpty(t, ot.SRDID)
		assert.Equal(t, "pending", ot.Status)
		assert.NotEmpty(t, ot.Items)
		assert.NotEmpty(t, ot.SRD.Problem)
	}
}

func TestBuildOutputTasks_Dependencies(t *testing.T) {
	corpus, err := spec.LoadCorpus(
		filepath.Join("..", "..", "..", "pkg", "spec", "testdata", "valid"))
	require.NoError(t, err)

	g, err := graph.BuildGraph(corpus)
	require.NoError(t, err)

	tasks := extractAllTasks(t, g)
	output := BuildOutputTasks(tasks, g, corpus)

	var apiTask *OutputTask
	for i := range output {
		if output[i].SRDID == "srd002-api" {
			apiTask = &output[i]
			break
		}
	}
	require.NotNil(t, apiTask, "should have an srd002-api task")
	assert.NotEmpty(t, apiTask.Deps, "srd002-api should depend on srd001-auth tasks")
}

func TestWriteReadTasks_RoundTrip(t *testing.T) {
	corpus, err := spec.LoadCorpus(
		filepath.Join("..", "..", "..", "pkg", "spec", "testdata", "valid"))
	require.NoError(t, err)

	g, err := graph.BuildGraph(corpus)
	require.NoError(t, err)

	tasks := extractAllTasks(t, g)
	output := BuildOutputTasks(tasks, g, corpus)

	path := filepath.Join(t.TempDir(), "tasks.yaml")
	require.NoError(t, WriteTasks(output, path, "testdata/valid", 100, "v0.test.1"))

	tf, err := ReadTasks(path)
	require.NoError(t, err)

	assert.Equal(t, len(output), len(tf.Tasks))
	assert.Equal(t, "testdata/valid", tf.Header.SourceDir)
	assert.Equal(t, 100, tf.Header.WeightThreshold)
	assert.Equal(t, len(output), tf.Header.TaskCount)
	assert.Equal(t, "v0.test.1", tf.Header.PlannerVersion)

	for i, ot := range output {
		assert.Equal(t, ot.ID, tf.Tasks[i].ID)
		assert.Equal(t, ot.SRDID, tf.Tasks[i].SRDID)
		assert.Equal(t, ot.Weight, tf.Tasks[i].Weight)
		assert.Equal(t, ot.Status, tf.Tasks[i].Status)
		assert.Equal(t, len(ot.Items), len(tf.Tasks[i].Items))
	}
}

func TestReadTasks_FileNotFound(t *testing.T) {
	_, err := ReadTasks("nonexistent.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read tasks")
}

func TestReadTasks_EmptyTasks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.yaml")
	require.NoError(t, os.WriteFile(path, []byte("tasks: []\n"), 0o644))

	_, err := ReadTasks(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no tasks")
}

func TestOutputTask_ItemIDs(t *testing.T) {
	corpus, err := spec.LoadCorpus(
		filepath.Join("..", "..", "..", "pkg", "spec", "testdata", "valid"))
	require.NoError(t, err)

	g, err := graph.BuildGraph(corpus)
	require.NoError(t, err)

	tasks := extractAllTasks(t, g)
	output := BuildOutputTasks(tasks, g, corpus)

	for _, ot := range output {
		for _, item := range ot.Items {
			assert.NotContains(t, item.ID, ot.SRDID+"-",
				"item ID %s should not contain SRD prefix", item.ID)
		}
	}
}
