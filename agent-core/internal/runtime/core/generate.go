// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// LoopSpec defines one iteration segment of an unrolled state machine.
// Each point in the experiment matrix becomes a linear chain of steps.
type LoopSpec struct {
	// Steps lists the tool actions to execute in order for this point.
	Steps []string `yaml:"steps"`
	// Vars holds point-specific variables (model, sample, etc.)
	// that tools can read from the state machine metadata.
	Vars map[string]string `yaml:"vars,omitempty"`
}

// GenerateSpec describes the input for generating a linear state machine.
type GenerateSpec struct {
	Name   string     `yaml:"name"`
	Points []LoopSpec `yaml:"points"`
	// DoneAction is the tool to run in the final Summarize state (optional).
	DoneAction string `yaml:"done_action,omitempty"`
}

// GenerateLinearMachine produces a flat MachineSpec from a GenerateSpec.
// Each point's steps become a linear chain of states with ToolDone transitions.
// ToolFailed transitions jump to the next point's first state (skip on failure).
//
// State naming: Point_<i>_Step_<j> where i is the point index and j is step index.
// Terminal state: Done.
func GenerateLinearMachine(gen GenerateSpec) MachineSpec {
	var (
		states      []string
		transitions []TransitionSpec
		signals     = []string{"Seed", "ToolDone", "ToolFailed"}
	)

	for i, point := range gen.Points {
		pointStates := make([]string, 0, len(point.Steps))
		for j := range point.Steps {
			name := fmt.Sprintf("Point_%d_Step_%d", i, j)
			pointStates = append(pointStates, name)
			states = append(states, name)
		}

		nextPointOrDone := "Done"
		if i+1 < len(gen.Points) {
			nextPointOrDone = fmt.Sprintf("Point_%d_Step_0", i+1)
		} else if gen.DoneAction != "" {
			nextPointOrDone = "Summarize"
		}

		for j, stepState := range pointStates {
			nextState := nextPointOrDone
			if j+1 < len(pointStates) {
				nextState = pointStates[j+1]
			}

			transitions = append(transitions, TransitionSpec{
				State:  stepState,
				Signal: "ToolDone",
				Next:   nextState,
				Action: point.Steps[j],
			})

			transitions = append(transitions, TransitionSpec{
				State:  stepState,
				Signal: "ToolFailed",
				Next:   nextPointOrDone,
				Action: "",
			})
		}
	}

	terminalStates := []string{"Done"}

	if gen.DoneAction != "" {
		states = append(states, "Summarize")
		transitions = append(transitions, TransitionSpec{
			State:  "Summarize",
			Signal: "ToolDone",
			Next:   "Done",
			Action: gen.DoneAction,
		})
		transitions = append(transitions, TransitionSpec{
			State:  "Summarize",
			Signal: "ToolFailed",
			Next:   "Done",
			Action: "",
		})
	}

	states = append(states, "Done")

	initial := "Done"
	if len(gen.Points) > 0 && len(gen.Points[0].Steps) > 0 {
		initial = "Point_0_Step_0"
	}

	// Seed transition to kick off from initial state
	seedTransitions := []TransitionSpec{{
		State:  initial,
		Signal: "Seed",
		Next:   initial,
		Action: gen.Points[0].Steps[0],
	}}

	allTransitions := append(seedTransitions, transitions...)

	return MachineSpec{
		Name:           gen.Name,
		InitialState:   initial,
		States:         StateSpecsFromNames(states...),
		TerminalStates: terminalStates,
		Signals:        SignalSpecsFromNames(signals...),
		Transitions:    allTransitions,
	}
}

// MarshalMachineSpec serializes a MachineSpec to YAML bytes.
func MarshalMachineSpec(spec MachineSpec) ([]byte, error) {
	return yaml.Marshal(spec)
}
