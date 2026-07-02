// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import "sort"

// --- Queries ---

// Nodes returns all nodes sorted by ID.
func (g *Graph) Nodes() []*Node {
	nodes := make([]*Node, 0, len(g.nodes))
	for _, n := range g.nodes {
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	return nodes
}

// NodesByKind returns all nodes of the given kind, sorted by ID.
func (g *Graph) NodesByKind(kind Kind) []*Node {
	var result []*Node
	for _, n := range g.nodes {
		if n.Kind == kind {
			result = append(result, n)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

// Node returns a single node by ID.
func (g *Graph) Node(id string) (*Node, bool) {
	n, ok := g.nodes[id]
	return n, ok
}

// NodeCount returns the total number of nodes.
func (g *Graph) NodeCount() int { return len(g.nodes) }

// Edges returns all edges sorted by source then target.
func (g *Graph) Edges() []Edge {
	edges, err := g.dag.Edges()
	if err != nil {
		return nil
	}
	var result []Edge
	for _, e := range edges {
		rel := ""
		if e.Properties.Attributes != nil {
			rel = e.Properties.Attributes["rel"]
		}
		result = append(result, Edge{Source: e.Source, Target: e.Target, Rel: rel})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Source != result[j].Source {
			return result[i].Source < result[j].Source
		}
		return result[i].Target < result[j].Target
	})
	return result
}

// EdgesByRel returns edges filtered by relationship label.
func (g *Graph) EdgesByRel(rel string) []Edge {
	var result []Edge
	for _, e := range g.Edges() {
		if e.Rel == rel {
			result = append(result, e)
		}
	}
	return result
}

// IncomingByRel returns node IDs with an edge of the given rel pointing to targetID.
func (g *Graph) IncomingByRel(targetID, rel string) []string {
	var result []string
	for _, e := range g.Edges() {
		if e.Target == targetID && e.Rel == rel {
			result = append(result, e.Source)
		}
	}
	sort.Strings(result)
	return result
}

// OutgoingByRel returns node IDs with an edge of the given rel from sourceID.
func (g *Graph) OutgoingByRel(sourceID, rel string) []string {
	var result []string
	for _, e := range g.Edges() {
		if e.Source == sourceID && e.Rel == rel {
			result = append(result, e.Target)
		}
	}
	sort.Strings(result)
	return result
}
