// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"fmt"
	"sort"
	"strings"

	dag "github.com/dominikbraun/graph"
)

// Kind labels the type of a node in the spec graph.
type Kind string

const (
	KindRelease           Kind = "release"
	KindSRD               Kind = "srd"
	KindReqGroup          Kind = "req-group"
	KindReqItem           Kind = "req-item"
	KindAC                Kind = "ac"
	KindUseCase           Kind = "use-case"
	KindTestSuite         Kind = "test-suite"
	KindTestCase          Kind = "test-case"
	KindMachine           Kind = "machine"
	KindMachineState      Kind = "machine-state"
	KindMachineSignal     Kind = "machine-signal"
	KindMachineTransition Kind = "machine-transition"
	KindToolDecl          Kind = "tool-decl"
)

// Edge relationship labels.
const (
	RelContains  = "contains"
	RelDependsOn = "depends-on"
	RelTraces    = "traces"
	RelTouches   = "touches"
	RelCites     = "cites"
	RelCovers    = "covers"
	RelAssigns   = "assigns"
	RelOrders    = "orders"
	RelSucceeds  = "succeeds"
	RelResolves  = "resolves"
)

// Node is a vertex in the labeled property graph.
type Node struct {
	ID   string `yaml:"id"`
	Kind Kind   `yaml:"kind"`

	// Fields populated depending on Kind.
	SRDID   string `yaml:"srd_id,omitempty"`
	Group   string `yaml:"group,omitempty"`
	Text    string `yaml:"text,omitempty"`
	Weight  int    `yaml:"weight,omitempty"`
	Release string `yaml:"release,omitempty"`
	Machine string `yaml:"machine,omitempty"`
}

// Edge is a labeled directed edge.
type Edge struct {
	Source string
	Target string
	Rel    string
}

// Graph is a labeled property graph over specification artifacts.
type Graph struct {
	dag   dag.Graph[string, *Node]
	nodes map[string]*Node
}

func nodeHash(n *Node) string { return n.ID }

// BuildGraph constructs a labeled property graph from a Corpus.
func BuildGraph(corpus *Corpus) (*Graph, error) {
	g := &Graph{
		dag:   dag.New(nodeHash, dag.Directed(), dag.Acyclic()),
		nodes: make(map[string]*Node),
	}

	srdRelease := buildSRDReleaseMap(corpus)

	if err := g.addReleaseNodes(corpus); err != nil {
		return nil, err
	}
	if err := g.addSRDNodes(corpus, srdRelease); err != nil {
		return nil, err
	}
	if err := g.addUseCaseNodes(corpus); err != nil {
		return nil, err
	}
	if err := g.addTestSuiteNodes(corpus); err != nil {
		return nil, err
	}
	if err := g.addMachineNodes(corpus); err != nil {
		return nil, err
	}
	if err := g.addToolDeclNodes(corpus); err != nil {
		return nil, err
	}

	if err := g.addReleaseEdges(corpus); err != nil {
		return nil, err
	}
	if err := g.addAssignEdges(corpus, srdRelease); err != nil {
		return nil, err
	}
	if err := g.addSRDContainsEdges(corpus); err != nil {
		return nil, err
	}
	if err := g.addIntraSRDEdges(corpus); err != nil {
		return nil, err
	}
	if err := g.addInterSRDEdges(corpus); err != nil {
		return nil, err
	}
	if err := g.addACTracesEdges(corpus); err != nil {
		return nil, err
	}
	if err := g.addUseCaseEdges(corpus); err != nil {
		return nil, err
	}
	if err := g.addTestCaseEdges(corpus); err != nil {
		return nil, err
	}
	if err := g.addMachineContainsEdges(corpus); err != nil {
		return nil, err
	}
	if err := g.addActionResolvesEdges(corpus); err != nil {
		return nil, err
	}

	return g, nil
}

func (g *Graph) addNode(n *Node) error {
	g.nodes[n.ID] = n
	return g.dag.AddVertex(n)
}

func (g *Graph) addEdge(from, to, rel string) error {
	err := g.dag.AddEdge(from, to, dag.EdgeAttribute("rel", rel))
	if err != nil && err.Error() == "edge already exists" {
		return nil
	}
	return err
}

// --- Node creation ---

func (g *Graph) addReleaseNodes(corpus *Corpus) error {
	for _, r := range corpus.Roadmap.Releases {
		if err := g.addNode(&Node{ID: "release:" + r.Version, Kind: KindRelease, Release: r.Version, Text: r.Name}); err != nil {
			return fmt.Errorf("add release node %s: %w", r.Version, err)
		}
	}
	return nil
}

func (g *Graph) addSRDNodes(corpus *Corpus, srdRelease map[string]string) error {
	for _, srdID := range corpus.SRDOrder {
		srd := corpus.SRDs[srdID]
		rel := srdRelease[srdID]

		if err := g.addNode(&Node{ID: srdID, Kind: KindSRD, Release: rel, Text: srd.Title}); err != nil {
			return fmt.Errorf("add SRD node %s: %w", srdID, err)
		}

		for _, gk := range srd.OrderedGroups {
			group := srd.Requirements[gk]
			groupID := srdID + ":" + gk
			if err := g.addNode(&Node{ID: groupID, Kind: KindReqGroup, SRDID: srdID, Group: gk, Text: group.Title, Release: rel}); err != nil {
				return fmt.Errorf("add req-group node %s: %w", groupID, err)
			}

			for _, item := range group.Items {
				itemID := srdID + ":" + item.ID
				if err := g.addNode(&Node{
					ID: itemID, Kind: KindReqItem, SRDID: srdID,
					Group: gk, Text: item.Text, Weight: item.Weight, Release: rel,
				}); err != nil {
					return fmt.Errorf("add req-item node %s: %w", itemID, err)
				}
			}
		}

		for _, ac := range srd.AcceptanceCriteria {
			acID := srdID + ":" + ac.ID
			if err := g.addNode(&Node{ID: acID, Kind: KindAC, SRDID: srdID, Text: ac.Criterion, Release: rel}); err != nil {
				return fmt.Errorf("add AC node %s: %w", acID, err)
			}
		}
	}
	return nil
}

func (g *Graph) addUseCaseNodes(corpus *Corpus) error {
	for _, ucID := range corpus.UCOrder {
		uc := corpus.UseCases[ucID]
		if err := g.addNode(&Node{ID: ucID, Kind: KindUseCase, Text: uc.Title}); err != nil {
			return fmt.Errorf("add use-case node %s: %w", ucID, err)
		}
	}
	return nil
}

func (g *Graph) addTestSuiteNodes(corpus *Corpus) error {
	for _, ts := range corpus.TestSuites {
		if err := g.addNode(&Node{ID: ts.ID, Kind: KindTestSuite, Release: ts.Release, Text: ts.Title}); err != nil {
			return fmt.Errorf("add test-suite node %s: %w", ts.ID, err)
		}

		for _, tc := range ts.TestCases {
			tcID := ts.ID + ":" + tc.Name
			if err := g.addNode(&Node{ID: tcID, Kind: KindTestCase, Text: tc.Description}); err != nil {
				return fmt.Errorf("add test-case node %s: %w", tcID, err)
			}
		}
	}
	return nil
}

func (g *Graph) addMachineNodes(corpus *Corpus) error {
	for _, agentName := range corpus.MachineOrder {
		ms := corpus.Machines[agentName]
		machineID := "machine:" + agentName
		if err := g.addNode(&Node{ID: machineID, Kind: KindMachine, Machine: agentName, Text: ms.Purpose}); err != nil {
			return fmt.Errorf("add machine node %s: %w", machineID, err)
		}
		for _, st := range ms.States {
			stateID := "machine-state:" + agentName + ":" + st.Name
			if err := g.addNode(&Node{ID: stateID, Kind: KindMachineState, Machine: agentName, Text: st.Meaning}); err != nil {
				return fmt.Errorf("add machine-state node %s: %w", stateID, err)
			}
		}
		for _, sig := range ms.Signals {
			sigID := "machine-signal:" + agentName + ":" + sig.Name
			if err := g.addNode(&Node{ID: sigID, Kind: KindMachineSignal, Machine: agentName, Text: sig.Trigger}); err != nil {
				return fmt.Errorf("add machine-signal node %s: %w", sigID, err)
			}
		}
		for _, tr := range ms.Transitions {
			trID := "machine-transition:" + agentName + ":" + tr.State + "+" + tr.Signal
			if err := g.addNode(&Node{ID: trID, Kind: KindMachineTransition, Machine: agentName, Text: tr.Action}); err != nil {
				return fmt.Errorf("add machine-transition node %s: %w", trID, err)
			}
		}
	}
	return nil
}

func (g *Graph) addToolDeclNodes(corpus *Corpus) error {
	names := make([]string, 0, len(corpus.ToolDeclarations))
	for name := range corpus.ToolDeclarations {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		td := corpus.ToolDeclarations[name]
		nodeID := "tool-decl:" + name
		if err := g.addNode(&Node{ID: nodeID, Kind: KindToolDecl, Text: td.Category}); err != nil {
			return fmt.Errorf("add tool-decl node %s: %w", nodeID, err)
		}
	}
	return nil
}

// --- Edge creation ---

func (g *Graph) addReleaseEdges(corpus *Corpus) error {
	versions := corpus.Roadmap.ReleaseVersions()
	for i := 0; i+1 < len(versions); i++ {
		if err := g.addEdge("release:"+versions[i], "release:"+versions[i+1], RelOrders); err != nil {
			return fmt.Errorf("release order edge: %w", err)
		}
	}
	return nil
}

func (g *Graph) addAssignEdges(corpus *Corpus, srdRelease map[string]string) error {
	for srdID, rel := range srdRelease {
		releaseNodeID := "release:" + rel
		if _, ok := g.nodes[releaseNodeID]; !ok {
			continue
		}
		if err := g.addEdge(releaseNodeID, srdID, RelAssigns); err != nil {
			return fmt.Errorf("assign edge %s -> %s: %w", rel, srdID, err)
		}
	}
	return nil
}

func (g *Graph) addSRDContainsEdges(corpus *Corpus) error {
	for _, srdID := range corpus.SRDOrder {
		srd := corpus.SRDs[srdID]
		for _, gk := range srd.OrderedGroups {
			groupID := srdID + ":" + gk
			if err := g.addEdge(srdID, groupID, RelContains); err != nil {
				return fmt.Errorf("srd contains group: %w", err)
			}

			group := srd.Requirements[gk]
			for _, item := range group.Items {
				itemID := srdID + ":" + item.ID
				if err := g.addEdge(groupID, itemID, RelContains); err != nil {
					return fmt.Errorf("group contains item: %w", err)
				}
			}
		}

		for _, ac := range srd.AcceptanceCriteria {
			acID := srdID + ":" + ac.ID
			if err := g.addEdge(srdID, acID, RelContains); err != nil {
				return fmt.Errorf("srd contains AC: %w", err)
			}
		}
	}
	return nil
}

func (g *Graph) addIntraSRDEdges(corpus *Corpus) error {
	for _, srdID := range corpus.SRDOrder {
		srd := corpus.SRDs[srdID]
		var prevLastItem string
		for _, gk := range srd.OrderedGroups {
			group := srd.Requirements[gk]
			if len(group.Items) == 0 {
				continue
			}

			firstItemID := srdID + ":" + group.Items[0].ID
			if prevLastItem != "" {
				if err := g.addEdge(prevLastItem, firstItemID, RelSucceeds); err != nil {
					return fmt.Errorf("inter-group succeeds: %w", err)
				}
			}

			for i := 0; i+1 < len(group.Items); i++ {
				from := srdID + ":" + group.Items[i].ID
				to := srdID + ":" + group.Items[i+1].ID
				if err := g.addEdge(from, to, RelSucceeds); err != nil {
					return fmt.Errorf("intra-group succeeds: %w", err)
				}
			}

			prevLastItem = srdID + ":" + group.Items[len(group.Items)-1].ID
		}
	}
	return nil
}

func (g *Graph) addInterSRDEdges(corpus *Corpus) error {
	for _, srdID := range corpus.SRDOrder {
		srd := corpus.SRDs[srdID]
		for _, dep := range srd.DependsOn {
			if _, ok := corpus.SRDs[dep.SRDID]; !ok {
				continue
			}
			if err := g.addEdge(srdID, dep.SRDID, RelDependsOn); err != nil {
				return fmt.Errorf("inter-SRD depends-on: %w", err)
			}
		}
	}
	return nil
}

func (g *Graph) addACTracesEdges(corpus *Corpus) error {
	for _, srdID := range corpus.SRDOrder {
		srd := corpus.SRDs[srdID]
		for _, ac := range srd.AcceptanceCriteria {
			acID := srdID + ":" + ac.ID
			for _, trace := range ac.Traces {
				itemID := srdID + ":" + trace
				if _, ok := g.nodes[itemID]; !ok {
					continue
				}
				if err := g.addEdge(acID, itemID, RelTraces); err != nil {
					return fmt.Errorf("AC traces: %w", err)
				}
			}
		}
	}
	return nil
}

func (g *Graph) addUseCaseEdges(corpus *Corpus) error {
	for _, ucID := range corpus.UCOrder {
		uc := corpus.UseCases[ucID]
		for _, tp := range uc.Touchpoints {
			srdID, groups := parseTouchpoint(tp)
			if srdID == "" {
				continue
			}
			if _, ok := g.nodes[srdID]; ok {
				_ = g.addEdge(ucID, srdID, RelTouches)
			}
			for _, grp := range groups {
				groupNodeID := srdID + ":" + grp
				if _, ok := g.nodes[groupNodeID]; ok {
					_ = g.addEdge(ucID, groupNodeID, RelCites)
				}
			}
		}

		for _, sc := range uc.SuccessCriteria {
			for _, trace := range sc.Traces {
				srdID, acID := parseACTrace(trace)
				if srdID == "" || acID == "" {
					continue
				}
				acNodeID := srdID + ":" + acID
				if _, ok := g.nodes[acNodeID]; ok {
					_ = g.addEdge(ucID, acNodeID, RelCites)
				}
			}
		}
	}
	return nil
}

func (g *Graph) addTestCaseEdges(corpus *Corpus) error {
	for _, ts := range corpus.TestSuites {
		for _, tc := range ts.TestCases {
			tcID := ts.ID + ":" + tc.Name
			for _, trace := range tc.Traces {
				srdID, acID := parseACTrace(trace)
				if srdID == "" || acID == "" {
					continue
				}
				acNodeID := srdID + ":" + acID
				if _, ok := g.nodes[acNodeID]; ok {
					_ = g.addEdge(tcID, acNodeID, RelCovers)
				}
			}
		}

		for _, ucID := range ts.Traces {
			if _, ok := g.nodes[ucID]; ok {
				_ = g.addEdge(ts.ID, ucID, RelCovers)
			}
		}
	}
	return nil
}

func (g *Graph) addMachineContainsEdges(corpus *Corpus) error {
	for _, agentName := range corpus.MachineOrder {
		ms := corpus.Machines[agentName]
		machineID := "machine:" + agentName
		for _, st := range ms.States {
			stateID := "machine-state:" + agentName + ":" + st.Name
			if err := g.addEdge(machineID, stateID, RelContains); err != nil {
				return fmt.Errorf("machine contains state: %w", err)
			}
		}
		for _, sig := range ms.Signals {
			sigID := "machine-signal:" + agentName + ":" + sig.Name
			if err := g.addEdge(machineID, sigID, RelContains); err != nil {
				return fmt.Errorf("machine contains signal: %w", err)
			}
		}
		for _, tr := range ms.Transitions {
			trID := "machine-transition:" + agentName + ":" + tr.State + "+" + tr.Signal
			if err := g.addEdge(machineID, trID, RelContains); err != nil {
				return fmt.Errorf("machine contains transition: %w", err)
			}
		}
	}
	return nil
}

func (g *Graph) addActionResolvesEdges(corpus *Corpus) error {
	for _, agentName := range corpus.MachineOrder {
		ms := corpus.Machines[agentName]
		for _, tr := range ms.Transitions {
			if tr.Action == "" {
				continue
			}
			trID := "machine-transition:" + agentName + ":" + tr.State + "+" + tr.Signal
			declID := "tool-decl:" + tr.Action
			if _, ok := g.nodes[declID]; ok {
				_ = g.addEdge(trID, declID, RelResolves)
			}
		}
	}
	return nil
}

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

// --- Touchpoint/trace parsing helpers ---

// parseTouchpoint extracts the SRD ID and cited R-groups from a touchpoint string.
// Formats: "srd005-cli R1 -- description" or "T1: srd005-cli R1 -- description".
func parseTouchpoint(tp string) (string, []string) {
	desc := strings.SplitN(tp, "--", 2)
	refs := trimTouchpointTag(desc[0])

	parts := strings.Fields(refs)
	if len(parts) == 0 {
		return "", nil
	}

	srdID := parts[0]
	if !strings.HasPrefix(srdID, "srd") {
		return "", nil
	}

	var groups []string
	for _, p := range parts[1:] {
		p = strings.TrimRight(p, ",")
		if p != "" && p[0] == 'R' {
			groups = append(groups, p)
		}
	}
	return srdID, groups
}

func trimTouchpointTag(refs string) string {
	refs = strings.TrimSpace(refs)
	label, rest, ok := strings.Cut(refs, ":")
	if !ok || !isTouchpointLabel(label) {
		return refs
	}
	return strings.TrimSpace(rest)
}

func isTouchpointLabel(label string) bool {
	if len(label) < 2 || label[0] != 'T' {
		return false
	}
	for _, r := range label[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// parseACTrace extracts SRD ID and AC ID from a trace string.
// Format: "srd005-cli-entry-point AC1"
func parseACTrace(trace string) (string, string) {
	parts := strings.Fields(trace)
	if len(parts) < 2 {
		return "", ""
	}
	srdID := parts[0]
	acID := parts[1]
	if !strings.HasPrefix(srdID, "srd") {
		return "", ""
	}
	return srdID, acID
}

// buildSRDReleaseMap parses the SPECIFICATIONS.yaml overview text
// for explicit release assignments like "- 00.0: srd001 (...), srd002 (...)".
// Short IDs (e.g. "srd001") are matched to full corpus IDs (e.g. "srd001-requirement-loader")
// by prefix.
func buildSRDReleaseMap(corpus *Corpus) map[string]string {
	m := make(map[string]string)
	overview := corpus.SpecIndex.Overview
	if overview == "" {
		return m
	}
	for _, line := range strings.Split(overview, "\n") {
		release, shortIDs := parseAssignmentLine(line)
		if release == "" {
			continue
		}
		for _, shortID := range shortIDs {
			fullID := resolveShortID(shortID, corpus.SRDOrder)
			if fullID == "" {
				continue
			}
			if _, exists := m[fullID]; !exists {
				m[fullID] = release
			}
		}
	}
	return m
}

// resolveShortID maps a potentially abbreviated SRD ID (e.g. "srd001")
// to the full corpus ID (e.g. "srd001-requirement-loader"). If the short
// ID already matches exactly, it's returned as-is.
func resolveShortID(short string, srdOrder []string) string {
	for _, full := range srdOrder {
		if full == short {
			return full
		}
		if strings.HasPrefix(full, short+"-") || strings.HasPrefix(full, short+"_") {
			return full
		}
	}
	return ""
}

func parseAssignmentLine(line string) (string, []string) {
	trimmed := strings.TrimLeft(line, " \t")
	if !strings.HasPrefix(trimmed, "- ") {
		return "", nil
	}
	trimmed = trimmed[2:]

	colonIdx := strings.IndexByte(trimmed, ':')
	if colonIdx < 1 {
		return "", nil
	}
	release := trimmed[:colonIdx]

	for _, c := range release {
		if c != '.' && (c < '0' || c > '9') {
			return "", nil
		}
	}

	rest := trimmed[colonIdx+1:]
	var ids []string
	for _, field := range strings.FieldsFunc(rest, func(r rune) bool {
		return r == ',' || r == '(' || r == ')'
	}) {
		field = strings.TrimSpace(field)
		if strings.HasPrefix(field, "srd") && len(field) > 3 {
			ids = append(ids, field)
		}
	}
	return release, ids
}
