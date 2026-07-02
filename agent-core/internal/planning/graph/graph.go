// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package graph

import (
	"fmt"
	"sort"

	dag "github.com/dominikbraun/graph"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/spec"
)

// Graph wraps a dominikbraun/graph DAG of requirement nodes with
// convenience methods for queries and state mutations.
type Graph struct {
	dag   dag.Graph[string, *Node]
	nodes map[string]*Node
}

// nodeHash returns the string ID used as the vertex key.
func nodeHash(n *Node) string { return n.ID }

// BuildGraph constructs a DAG from a Corpus. It creates nodes for every
// requirement item and adds three kinds of edges:
//   - Intra-SRD: consecutive items and consecutive groups within each SRD
//   - Inter-SRD: depends_on edges between SRDs
//   - Release: ordering edges between releases
func BuildGraph(corpus *spec.Corpus) (*Graph, error) {
	g := &Graph{
		dag:   dag.New(nodeHash, dag.Directed(), dag.Acyclic()),
		nodes: make(map[string]*Node),
	}

	srdRelease := buildSRDReleaseMap(corpus)

	for _, srdID := range corpus.SRDOrder {
		srd := corpus.SRDs[srdID]
		if err := g.addSRDNodes(srd, srdRelease[srdID]); err != nil {
			return nil, fmt.Errorf("add nodes for %s: %w", srdID, err)
		}
	}

	for _, srdID := range corpus.SRDOrder {
		srd := corpus.SRDs[srdID]
		if err := g.addIntraSRDEdges(srd); err != nil {
			return nil, fmt.Errorf("intra-SRD edges for %s: %w", srdID, err)
		}
	}

	if err := g.addInterSRDEdges(corpus); err != nil {
		return nil, fmt.Errorf("inter-SRD edges: %w", err)
	}

	if err := g.addReleaseEdges(corpus, srdRelease); err != nil {
		return nil, fmt.Errorf("release edges: %w", err)
	}

	return g, nil
}

func buildSRDReleaseMap(corpus *spec.Corpus) map[string]string {
	m := make(map[string]string)
	for _, entry := range corpus.SpecIndex.RoadmapSummary {
		_ = entry
	}

	overview := corpus.SpecIndex.Overview
	if overview != "" {
		parseReleaseAssignments(overview, m)
	}

	return m
}

// parseReleaseAssignments extracts SRD-to-release mappings from the
// SPECIFICATIONS.yaml overview text. It looks for patterns like:
//
//   - 00.0: srd001 (...), srd002 (...)
//   - 00.1: srd003 (...)
func parseReleaseAssignments(overview string, m map[string]string) {
	lines := splitLines(overview)
	for _, line := range lines {
		release, srdIDs := parseAssignmentLine(line)
		if release == "" {
			continue
		}
		for _, id := range srdIDs {
			if _, exists := m[id]; !exists {
				m[id] = release
			}
		}
	}
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func parseAssignmentLine(line string) (string, []string) {
	trimmed := line
	for len(trimmed) > 0 && (trimmed[0] == ' ' || trimmed[0] == '\t') {
		trimmed = trimmed[1:]
	}
	if len(trimmed) < 2 || trimmed[0] != '-' || trimmed[1] != ' ' {
		return "", nil
	}
	trimmed = trimmed[2:]

	colonIdx := -1
	for i, c := range trimmed {
		if c == ':' {
			colonIdx = i
			break
		}
	}
	if colonIdx < 1 {
		return "", nil
	}
	release := trimmed[:colonIdx]

	hasDigit := false
	for _, c := range release {
		if c >= '0' && c <= '9' {
			hasDigit = true
		} else if c != '.' {
			return "", nil
		}
	}
	if !hasDigit {
		return "", nil
	}

	rest := trimmed[colonIdx+1:]
	return release, extractSRDIDs(rest)
}

func extractSRDIDs(s string) []string {
	var ids []string
	i := 0
	for i < len(s) {
		idx := indexOfSRD(s[i:])
		if idx < 0 {
			break
		}
		start := i + idx
		end := start
		for end < len(s) && s[end] != ',' && s[end] != ' ' && s[end] != ')' && s[end] != '\n' {
			end++
		}
		id := s[start:end]
		if len(id) > 3 {
			ids = append(ids, id)
		}
		i = end + 1
	}
	return ids
}

func indexOfSRD(s string) int {
	for i := 0; i+3 <= len(s); i++ {
		if s[i] == 's' && s[i+1] == 'r' && s[i+2] == 'd' {
			return i
		}
	}
	return -1
}

func (g *Graph) addSRDNodes(srd spec.SRD, release string) error {
	for _, gk := range srd.OrderedGroups {
		group := srd.Requirements[gk]
		for _, item := range group.Items {
			nodeID := srd.ID + "-" + item.ID
			n := &Node{
				ID:      nodeID,
				SRDID:   srd.ID,
				Group:   gk,
				Text:    item.Text,
				Weight:  item.Weight,
				Release: release,
				Status:  Pending,
				Retries: 0,
			}
			g.nodes[nodeID] = n
			if err := g.dag.AddVertex(n); err != nil {
				return fmt.Errorf("add vertex %s: %w", nodeID, err)
			}
		}
	}
	return nil
}

func (g *Graph) addIntraSRDEdges(srd spec.SRD) error {
	var prevLast string
	for _, gk := range srd.OrderedGroups {
		group := srd.Requirements[gk]
		if len(group.Items) == 0 {
			continue
		}

		firstID := srd.ID + "-" + group.Items[0].ID
		if prevLast != "" {
			if err := g.dag.AddEdge(prevLast, firstID); err != nil {
				return fmt.Errorf("inter-group edge %s → %s: %w", prevLast, firstID, err)
			}
		}

		for i := 0; i+1 < len(group.Items); i++ {
			from := srd.ID + "-" + group.Items[i].ID
			to := srd.ID + "-" + group.Items[i+1].ID
			if err := g.dag.AddEdge(from, to); err != nil {
				return fmt.Errorf("intra-group edge %s → %s: %w", from, to, err)
			}
		}

		prevLast = srd.ID + "-" + group.Items[len(group.Items)-1].ID
	}
	return nil
}

func (g *Graph) addInterSRDEdges(corpus *spec.Corpus) error {
	for _, srdID := range corpus.SRDOrder {
		srd := corpus.SRDs[srdID]
		for _, dep := range srd.DependsOn {
			depSRD, ok := corpus.SRDs[dep.SRDID]
			if !ok {
				continue
			}
			lastOfDep := lastNodeID(depSRD)
			firstOfDependent := firstNodeID(srd)
			if lastOfDep == "" || firstOfDependent == "" {
				continue
			}
			if err := g.dag.AddEdge(lastOfDep, firstOfDependent); err != nil {
				if isEdgeAlreadyExists(err) {
					continue
				}
				return fmt.Errorf("inter-SRD edge %s → %s: %w", lastOfDep, firstOfDependent, err)
			}
		}
	}
	return nil
}

func (g *Graph) addReleaseEdges(corpus *spec.Corpus, srdRelease map[string]string) error {
	releases := corpus.Roadmap.ReleaseVersions()
	if len(releases) < 2 {
		return nil
	}

	byRelease := make(map[string][]string)
	for _, srdID := range corpus.SRDOrder {
		rel := srdRelease[srdID]
		if rel != "" {
			byRelease[rel] = append(byRelease[rel], srdID)
		}
	}

	for i := 0; i+1 < len(releases); i++ {
		currRelease := releases[i]
		nextRelease := releases[i+1]

		currSRDs := byRelease[currRelease]
		nextSRDs := byRelease[nextRelease]
		if len(currSRDs) == 0 || len(nextSRDs) == 0 {
			continue
		}

		for _, currSRDID := range currSRDs {
			currSRD := corpus.SRDs[currSRDID]
			last := lastNodeID(currSRD)
			if last == "" {
				continue
			}
			for _, nextSRDID := range nextSRDs {
				nextSRD := corpus.SRDs[nextSRDID]
				first := firstNodeID(nextSRD)
				if first == "" {
					continue
				}
				if err := g.dag.AddEdge(last, first); err != nil {
					if isEdgeAlreadyExists(err) {
						continue
					}
					return fmt.Errorf("release edge %s → %s: %w", last, first, err)
				}
			}
		}
	}
	return nil
}

func firstNodeID(srd spec.SRD) string {
	if len(srd.OrderedGroups) == 0 {
		return ""
	}
	g := srd.Requirements[srd.OrderedGroups[0]]
	if len(g.Items) == 0 {
		return ""
	}
	return srd.ID + "-" + g.Items[0].ID
}

func lastNodeID(srd spec.SRD) string {
	if len(srd.OrderedGroups) == 0 {
		return ""
	}
	lastGroup := srd.Requirements[srd.OrderedGroups[len(srd.OrderedGroups)-1]]
	if len(lastGroup.Items) == 0 {
		return ""
	}
	return srd.ID + "-" + lastGroup.Items[len(lastGroup.Items)-1].ID
}

func isEdgeAlreadyExists(err error) bool {
	return err != nil && err.Error() == "edge already exists"
}

// --- Queries ---

// Nodes returns all nodes in the graph sorted by ID.
func (g *Graph) Nodes() []*Node {
	nodes := make([]*Node, 0, len(g.nodes))
	for _, n := range g.nodes {
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	return nodes
}

// Node returns a single node by ID.
func (g *Graph) Node(id string) (*Node, bool) {
	n, ok := g.nodes[id]
	return n, ok
}

// NodeCount returns the total number of nodes.
func (g *Graph) NodeCount() int { return len(g.nodes) }

// TopologicalSort returns node IDs in a valid topological order.
func (g *Graph) TopologicalSort() ([]string, error) {
	order, err := dag.StableTopologicalSort(g.dag, func(s1, s2 string) bool {
		return s1 < s2
	})
	if err != nil {
		return nil, fmt.Errorf("topological sort: %w", err)
	}
	return order, nil
}

// Ready returns all nodes whose predecessors are all Done and whose
// own status is Pending.
func (g *Graph) Ready() []*Node {
	adjMap, err := g.dag.AdjacencyMap()
	if err != nil {
		return nil
	}
	predMap, err := g.dag.PredecessorMap()
	if err != nil {
		return nil
	}

	var ready []*Node
	for id := range adjMap {
		node := g.nodes[id]
		if node.Status != Pending {
			continue
		}
		preds := predMap[id]
		allDone := true
		for predID := range preds {
			if g.nodes[predID].Status != Done {
				allDone = false
				break
			}
		}
		if allDone {
			ready = append(ready, node)
		}
	}

	sort.Slice(ready, func(i, j int) bool { return ready[i].ID < ready[j].ID })
	return ready
}

// Blocked returns all nodes that have at least one predecessor in
// status Failed.
func (g *Graph) Blocked() []*Node {
	predMap, err := g.dag.PredecessorMap()
	if err != nil {
		return nil
	}

	var blocked []*Node
	for id, preds := range predMap {
		node := g.nodes[id]
		if node.Status == Done || node.Status == Failed {
			continue
		}
		for predID := range preds {
			if g.nodes[predID].Status == Failed {
				blocked = append(blocked, node)
				break
			}
		}
	}

	sort.Slice(blocked, func(i, j int) bool { return blocked[i].ID < blocked[j].ID })
	return blocked
}

// Predecessors returns the direct predecessors of a node.
func (g *Graph) Predecessors(nodeID string) ([]*Node, error) {
	predMap, err := g.dag.PredecessorMap()
	if err != nil {
		return nil, err
	}
	preds, ok := predMap[nodeID]
	if !ok {
		return nil, fmt.Errorf("node %s not found", nodeID)
	}

	var result []*Node
	for predID := range preds {
		result = append(result, g.nodes[predID])
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

// DFSFrom returns nodes reachable from startID in DFS order,
// filtered to only Ready nodes (Pending with all predecessors Done).
func (g *Graph) DFSFrom(startID string) []*Node {
	readySet := make(map[string]bool)
	for _, n := range g.Ready() {
		readySet[n.ID] = true
	}

	var result []*Node
	_ = dag.DFS(g.dag, startID, func(id string) bool {
		if readySet[id] {
			result = append(result, g.nodes[id])
		}
		return false
	})
	return result
}

// Edges returns all edges as (source, target) pairs, sorted.
func (g *Graph) Edges() [][2]string {
	edges, err := g.dag.Edges()
	if err != nil {
		return nil
	}
	var result [][2]string
	for _, e := range edges {
		result = append(result, [2]string{e.Source, e.Target})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i][0] != result[j][0] {
			return result[i][0] < result[j][0]
		}
		return result[i][1] < result[j][1]
	})
	return result
}
