// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

// Package graph builds and maintains a dependency DAG of requirement
// items using dominikbraun/graph.
package graph

import "fmt"

// Status represents the lifecycle state of a requirement node.
type Status string

const (
	Pending   Status = "pending"
	Planning  Status = "planning"
	Executing Status = "executing"
	Done      Status = "done"
	Failed    Status = "failed"
)

// Node represents a single requirement item in the dependency graph.
type Node struct {
	ID      string `yaml:"id"`
	SRDID   string `yaml:"srd_id"`
	Group   string `yaml:"group"`
	Text    string `yaml:"text"`
	Weight  int    `yaml:"weight"`
	Release string `yaml:"release"`

	Status Status `yaml:"status"`
}

// validTransitions defines the allowed state machine transitions.
var validTransitions = map[Status][]Status{
	Pending:   {Planning},
	Planning:  {Executing},
	Executing: {Done, Failed},
}

// MarkPlanning transitions the node to Planning. Only valid from Pending.
func (n *Node) MarkPlanning() error {
	return n.transition(Planning)
}

// MarkExecuting transitions the node to Executing. Only valid from Planning.
func (n *Node) MarkExecuting() error {
	return n.transition(Executing)
}

// MarkDone transitions the node to Done. Only valid from Executing.
func (n *Node) MarkDone() error {
	return n.transition(Done)
}

// MarkFailed transitions the node to Failed. Retry policy belongs to
// machine-visible command state, not graph nodes.
func (n *Node) MarkFailed() error {
	return n.transition(Failed)
}

func (n *Node) transition(target Status) error {
	for _, allowed := range validTransitions[n.Status] {
		if allowed == target {
			n.Status = target
			return nil
		}
	}
	return fmt.Errorf("invalid transition %s → %s for node %s", n.Status, target, n.ID)
}
