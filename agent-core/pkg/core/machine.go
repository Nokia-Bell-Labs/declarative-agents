// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// MachineSpec is the YAML schema for a declarative state machine.
type MachineSpec struct {
	Name           string           `yaml:"name"`
	InitialState   string           `yaml:"initial_state"`
	States         []string         `yaml:"states"`
	TerminalStates []string         `yaml:"terminal_states"`
	Signals        []string         `yaml:"signals"`
	Transitions    []TransitionSpec `yaml:"transitions"`
	BudgetSpec     *BudgetSpec      `yaml:"budget,omitempty"`
}

// BudgetSpec is the optional budget block in machine YAML.
// Zero values mean "use default" or "unlimited".
type BudgetSpec struct {
	MaxIterations            int    `yaml:"max_iterations,omitempty"`
	MaxTokens                int    `yaml:"max_tokens,omitempty"`
	MaxDuration              string `yaml:"max_duration,omitempty"`
	MaxConsecutiveParseErrors int    `yaml:"max_consecutive_parse_errors,omitempty"`
}

// ToBudget converts a BudgetSpec into a Budget, applying defaults.
func (bs *BudgetSpec) ToBudget(defaults Budget) Budget {
	b := defaults
	if bs == nil {
		return b
	}
	if bs.MaxIterations > 0 {
		b.MaxIterations = bs.MaxIterations
	}
	if bs.MaxTokens > 0 {
		b.MaxTokens = bs.MaxTokens
	}
	if bs.MaxDuration != "" {
		if d, err := time.ParseDuration(bs.MaxDuration); err == nil {
			b.MaxDuration = d
		}
	}
	return b
}

// TransitionSpec is one row in the transition table YAML.
// Action is either a tool name (resolved from the registry) or "$tool"
// for dynamic dispatch. Empty action means terminal (no command).
type TransitionSpec struct {
	State  string `yaml:"state"`
	Signal string `yaml:"signal"`
	Next   string `yaml:"next"`
	Action string `yaml:"action,omitempty"`
}

// LoadMachineSpec reads and parses a machine YAML file.
func LoadMachineSpec(path string) (MachineSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return MachineSpec{}, fmt.Errorf("read machine spec %s: %w", path, err)
	}
	return ParseMachineSpec(data)
}

// ParseMachineSpec parses machine YAML from bytes.
func ParseMachineSpec(data []byte) (MachineSpec, error) {
	var spec MachineSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return MachineSpec{}, fmt.Errorf("parse machine YAML: %w", err)
	}
	if err := validateSpec(spec); err != nil {
		return MachineSpec{}, err
	}
	return spec, nil
}

func validateSpec(spec MachineSpec) error {
	var errs []string

	if spec.InitialState == "" {
		errs = append(errs, "initial_state is required")
	}
	if len(spec.States) == 0 {
		errs = append(errs, "at least one state is required")
	}
	if len(spec.TerminalStates) == 0 {
		errs = append(errs, "at least one terminal_state is required")
	}
	if len(spec.Transitions) == 0 {
		errs = append(errs, "at least one transition is required")
	}

	stateSet := make(map[string]bool)
	for _, s := range spec.States {
		stateSet[s] = true
	}

	if spec.InitialState != "" && !stateSet[spec.InitialState] {
		errs = append(errs, fmt.Sprintf("initial_state %q not in states list", spec.InitialState))
	}
	for _, ts := range spec.TerminalStates {
		if !stateSet[ts] {
			errs = append(errs, fmt.Sprintf("terminal_state %q not in states list", ts))
		}
	}

	signalSet := make(map[string]bool)
	for _, s := range spec.Signals {
		signalSet[s] = true
	}

	for i, tr := range spec.Transitions {
		if !stateSet[tr.State] {
			errs = append(errs, fmt.Sprintf("transition[%d]: state %q not in states list", i, tr.State))
		}
		if !signalSet[tr.Signal] {
			errs = append(errs, fmt.Sprintf("transition[%d]: signal %q not in signals list", i, tr.Signal))
		}
		if !stateSet[tr.Next] {
			errs = append(errs, fmt.Sprintf("transition[%d]: next %q not in states list", i, tr.Next))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("machine spec validation: %s", strings.Join(errs, "; "))
	}
	return nil
}

// BuildTransitionTable converts a MachineSpec into a core.TransitionTable
// using the provided registry to resolve action names to builders.
// The special action "$tool" is resolved using the provided toolAction.
// Empty actions produce nil ActionFuncs (terminal transitions).
func BuildTransitionTable(spec MachineSpec, reg *Registry, toolAction ActionFunc) (TransitionTable, TerminalFunc, error) {
	terminalSet := make(map[State]bool)
	for _, ts := range spec.TerminalStates {
		terminalSet[State(ts)] = true
	}

	isTerminal := func(s State) bool {
		return terminalSet[s]
	}

	table := make(TransitionTable)
	for i, tr := range spec.Transitions {
		key := TransitionInput{
			State:  State(tr.State),
			Signal: Signal(tr.Signal),
		}

		var action ActionFunc
		switch {
		case tr.Action == "":
			action = nil
		case tr.Action == "$tool":
			if toolAction == nil {
				return nil, nil, fmt.Errorf("transition[%d]: $tool action requires a toolAction function", i)
			}
			action = toolAction
		default:
			builder, ok := reg.Resolve(tr.Action)
			if !ok {
				return nil, nil, fmt.Errorf("transition[%d]: action %q not found in registry", i, tr.Action)
			}
			b := builder
			action = func(r Result) Command { return b.Build(r) }
		}

		table[key] = TransitionValue{
			NextState: State(tr.Next),
			Action:    action,
		}
	}

	return table, isTerminal, nil
}
