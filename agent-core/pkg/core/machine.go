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
	MaxIterations             int    `yaml:"max_iterations,omitempty"`
	MaxTokens                 int    `yaml:"max_tokens,omitempty"`
	MaxDuration               string `yaml:"max_duration,omitempty"`
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

// MachineDiagnosticSeverity classifies non-fatal grammar diagnostics.
type MachineDiagnosticSeverity string

const (
	MachineDiagnosticWarning MachineDiagnosticSeverity = "warning"
)

// MachineDiagnostic describes a static grammar issue that does not make the
// machine structurally invalid, but may indicate dead or surprising grammar.
type MachineDiagnostic struct {
	Severity        MachineDiagnosticSeverity
	Code            string
	Message         string
	State           string
	Signal          string
	TransitionIndex int
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

// DiagnoseMachineSpec reports reachability and dead-grammar diagnostics for a
// structurally valid machine. It is intentionally non-fatal so callers can
// decide which warnings are policy violations for their workflow.
func DiagnoseMachineSpec(spec MachineSpec) []MachineDiagnostic {
	reachable := reachableStates(spec)
	usedSignals := make(map[string]bool)
	terminalSet := make(map[string]bool, len(spec.TerminalStates))
	for _, state := range spec.TerminalStates {
		terminalSet[state] = true
	}

	var diagnostics []MachineDiagnostic
	for _, state := range spec.States {
		if state == spec.InitialState {
			continue
		}
		if !reachable[state] {
			diagnostics = append(diagnostics, MachineDiagnostic{
				Severity: MachineDiagnosticWarning,
				Code:     "unreachable_state",
				Message:  fmt.Sprintf("state %q is not reachable from initial_state %q", state, spec.InitialState),
				State:    state,
			})
		}
	}

	for i, tr := range spec.Transitions {
		usedSignals[tr.Signal] = true
		if !reachable[tr.State] {
			diagnostics = append(diagnostics, MachineDiagnostic{
				Severity:        MachineDiagnosticWarning,
				Code:            "unreachable_transition",
				Message:         fmt.Sprintf("transition[%d] from %s/%s is unreachable", i, tr.State, tr.Signal),
				State:           tr.State,
				Signal:          tr.Signal,
				TransitionIndex: i,
			})
		}
		if terminalSet[tr.State] {
			diagnostics = append(diagnostics, MachineDiagnostic{
				Severity:        MachineDiagnosticWarning,
				Code:            "terminal_transition",
				Message:         fmt.Sprintf("transition[%d] starts from terminal state %q", i, tr.State),
				State:           tr.State,
				Signal:          tr.Signal,
				TransitionIndex: i,
			})
		}
	}

	for _, signal := range spec.Signals {
		if !usedSignals[signal] {
			diagnostics = append(diagnostics, MachineDiagnostic{
				Severity: MachineDiagnosticWarning,
				Code:     "unused_signal",
				Message:  fmt.Sprintf("signal %q is declared but no transition uses it", signal),
				Signal:   signal,
			})
		}
	}

	return diagnostics
}

func reachableStates(spec MachineSpec) map[string]bool {
	reachable := map[string]bool{}
	if spec.InitialState == "" {
		return reachable
	}
	adjacency := make(map[string][]string, len(spec.States))
	for _, tr := range spec.Transitions {
		adjacency[tr.State] = append(adjacency[tr.State], tr.Next)
	}
	queue := []string{spec.InitialState}
	reachable[spec.InitialState] = true
	for len(queue) > 0 {
		state := queue[0]
		queue = queue[1:]
		for _, next := range adjacency[state] {
			if reachable[next] {
				continue
			}
			reachable[next] = true
			queue = append(queue, next)
		}
	}
	return reachable
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
