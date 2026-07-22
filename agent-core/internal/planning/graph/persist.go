// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package graph

import (
	"fmt"
	"os"
	"sort"

	dag "github.com/dominikbraun/graph"
	"gopkg.in/yaml.v3"
)

// GraphState is the serializable representation of a Graph.
type GraphState struct {
	Nodes []Node `yaml:"nodes"`
	Edges []Edge `yaml:"edges"`
}

// Edge is a serializable directed edge.
type Edge struct {
	Source string `yaml:"source"`
	Target string `yaml:"target"`
}

// SaveGraph writes the graph state to a YAML file. Nodes are sorted by
// ID for deterministic, human-readable, git-diffable output.
func SaveGraph(g *Graph, path string) error {
	nodes := g.Nodes()
	state := GraphState{
		Nodes: make([]Node, len(nodes)),
	}
	for i, n := range nodes {
		state.Nodes[i] = *n
	}

	for _, e := range g.Edges() {
		state.Edges = append(state.Edges, Edge{Source: e[0], Target: e[1]})
	}

	data, err := yaml.Marshal(&state)
	if err != nil {
		return fmt.Errorf("marshal graph state: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write graph state to %s: %w", path, err)
	}
	return nil
}

// LoadGraph reads a graph state from a YAML file and reconstructs the
// dominikbraun/graph instance with all nodes, edges, and per-node state.
func LoadGraph(path string) (*Graph, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read graph state from %s: %w", path, err)
	}

	var state GraphState
	if err := yaml.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshal graph state: %w", err)
	}
	if err := validateGraphState(state); err != nil {
		return nil, fmt.Errorf("validate graph state: %w", err)
	}

	g := &Graph{
		dag:   dag.New(nodeHash, dag.Directed(), dag.Acyclic()),
		nodes: make(map[string]*Node, len(state.Nodes)),
	}

	sort.Slice(state.Nodes, func(i, j int) bool {
		return state.Nodes[i].ID < state.Nodes[j].ID
	})

	for i := range state.Nodes {
		n := &state.Nodes[i]
		g.nodes[n.ID] = n
		if err := g.dag.AddVertex(n); err != nil {
			return nil, fmt.Errorf("restore vertex %s: %w", n.ID, err)
		}
	}

	for _, e := range state.Edges {
		if err := g.dag.AddEdge(e.Source, e.Target); err != nil {
			return nil, fmt.Errorf("restore edge %s → %s: %w", e.Source, e.Target, err)
		}
	}

	return g, nil
}

func validateGraphState(state GraphState) error {
	nodeIDs := make(map[string]int, len(state.Nodes))
	validStatuses := map[Status]bool{
		Pending: true, Planning: true, Executing: true, Done: true, Failed: true,
	}
	for i, node := range state.Nodes {
		if node.ID == "" {
			return fmt.Errorf("nodes[%d].id is required", i)
		}
		if first, exists := nodeIDs[node.ID]; exists {
			return fmt.Errorf("nodes[%d].id %q duplicates nodes[%d].id", i, node.ID, first)
		}
		nodeIDs[node.ID] = i
		if node.SRDID == "" {
			return fmt.Errorf("nodes[%d] %q srd_id is required", i, node.ID)
		}
		if !validStatuses[node.Status] {
			return fmt.Errorf("nodes[%d] %q status %q is invalid", i, node.ID, node.Status)
		}
		if node.Retries < 0 {
			return fmt.Errorf("nodes[%d] %q retries must be nonnegative", i, node.ID)
		}
		if node.Status == Failed && node.Retries == 0 {
			return fmt.Errorf("nodes[%d] %q failed status requires at least one retry", i, node.ID)
		}
	}
	for i, edge := range state.Edges {
		if edge.Source == "" {
			return fmt.Errorf("edges[%d].source is required", i)
		}
		if edge.Target == "" {
			return fmt.Errorf("edges[%d].target is required", i)
		}
		if _, exists := nodeIDs[edge.Source]; !exists {
			return fmt.Errorf("edges[%d].source %q does not reference a node", i, edge.Source)
		}
		if _, exists := nodeIDs[edge.Target]; !exists {
			return fmt.Errorf("edges[%d].target %q does not reference a node", i, edge.Target)
		}
	}
	return nil
}
