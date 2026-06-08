// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// Package extract selects weight-bounded tasks from the dependency graph.
// Implements srd003-task-extraction.
package extract

import (
	"fmt"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/graph"
)

const defaultMaxWeight = 5

// Task is a weight-bounded group of contiguous requirement nodes from
// a single SRD.
type Task struct {
	ID      string   `yaml:"id"`
	NodeIDs []string `yaml:"node_ids"`
	Weight  int      `yaml:"weight"`
	SRDID   string   `yaml:"srd_id"`
	Release string   `yaml:"release"`
}

// Extractor selects tasks from the dependency graph using DFS traversal
// with SRD affinity.
type Extractor struct {
	hintSRD string
}

// NewExtractor creates an Extractor with no initial SRD hint.
func NewExtractor() *Extractor {
	return &Extractor{}
}

// ExtractNext selects the next task from the graph. Returns nil when no
// ready nodes exist. maxWeight is the weight threshold (0 means unlimited,
// negative uses the default of 5).
func (e *Extractor) ExtractNext(g *graph.Graph, maxWeight int) *Task {
	if maxWeight < 0 {
		maxWeight = defaultMaxWeight
	}

	ready := g.Ready()
	if len(ready) == 0 {
		return nil
	}

	start := e.selectStart(ready)
	nodes := e.accumulate(g, start, maxWeight)

	if len(nodes) == 0 {
		return nil
	}

	task := buildTask(nodes)
	e.hintSRD = task.SRDID
	return task
}

// selectStart picks the best starting node from the ready set using
// DFS affinity: prefer the hint SRD, then lowest release/SRD.
func (e *Extractor) selectStart(ready []*graph.Node) *graph.Node {
	if e.hintSRD != "" {
		for _, n := range ready {
			if n.SRDID == e.hintSRD {
				return n
			}
		}
	}
	// ready is already sorted by ID (which sorts by release, SRD, group, item)
	return ready[0]
}

// accumulate collects contiguous same-SRD nodes starting from the given
// node, respecting the weight threshold.
func (e *Extractor) accumulate(g *graph.Graph, start *graph.Node, maxWeight int) []*graph.Node {
	srdID := start.SRDID

	ready := g.Ready()
	var srdReady []*graph.Node
	for _, n := range ready {
		if n.SRDID == srdID {
			srdReady = append(srdReady, n)
		}
	}

	var result []*graph.Node
	totalWeight := 0

	for _, n := range srdReady {
		newWeight := totalWeight + n.Weight
		if maxWeight > 0 && newWeight > maxWeight && len(result) > 0 {
			break
		}
		result = append(result, n)
		totalWeight = newWeight
	}

	return result
}

func buildTask(nodes []*graph.Node) *Task {
	nodeIDs := make([]string, len(nodes))
	totalWeight := 0
	for i, n := range nodes {
		nodeIDs[i] = n.ID
		totalWeight += n.Weight
	}

	first := nodes[0]
	last := nodes[len(nodes)-1]

	taskID := fmt.Sprintf("%s..%s", first.ID, last.ID)
	if len(nodes) == 1 {
		taskID = first.ID
	}

	return &Task{
		ID:      taskID,
		NodeIDs: nodeIDs,
		Weight:  totalWeight,
		SRDID:   first.SRDID,
		Release: first.Release,
	}
}

// HintSRD returns the current traversal hint (last SRD worked on).
func (e *Extractor) HintSRD() string {
	return e.hintSRD
}
