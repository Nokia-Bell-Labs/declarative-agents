// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import dag "github.com/dominikbraun/graph"

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
