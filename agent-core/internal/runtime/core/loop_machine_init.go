// Copyright (c) 2026 Nokia. All rights reserved.

package core

import "fmt"

// initFromMachine handles declarative initialization when MachineFile or
// MachineSpec is set. It creates the registry, calls InitFunc, loads or reuses
// the machine, validates actions, and populates Table/IsTerminal/InitialState.
func initFromMachine(params *LoopParams) error {
	if params.MachineFile == "" && params.MachineSpec == nil {
		return nil
	}
	if params.Registry == nil {
		params.Registry = NewRegistry()
	}
	if params.InitFunc != nil {
		if err := params.InitFunc(params.Registry); err != nil {
			return fmt.Errorf("init tools: %w", err)
		}
	}
	spec, err := loopMachineSpec(params)
	if err != nil {
		return err
	}
	table, isTerminal, err := BuildTransitionTable(spec, params.Registry, params.ToolAction)
	if err != nil {
		return err
	}
	params.Table = table
	params.IsTerminal = isTerminal
	if params.InitialState == "" {
		params.InitialState = State(spec.InitialState)
	}
	params.MachineSpec = &spec
	return nil
}

func loopMachineSpec(params *LoopParams) (MachineSpec, error) {
	if params.MachineSpec != nil {
		return *params.MachineSpec, nil
	}
	return LoadMachineSpec(params.MachineFile)
}

func transitionMetricLabels(spec *MachineSpec, state State, signal Signal) MetricLabels {
	if spec == nil {
		return nil
	}
	labels := copyMetricLabels(spec.MetricLabels)
	for _, tr := range spec.Transitions {
		if tr.State == string(state) && tr.Signal == string(signal) {
			mergeMetricLabels(labels, tr.MetricLabels)
			return labels
		}
	}
	return labels
}

func copyMetricLabels(labels MetricLabels) MetricLabels {
	out := make(MetricLabels, len(labels))
	for name, value := range labels {
		out[name] = value
	}
	return out
}

func mergeMetricLabels(dst MetricLabels, src MetricLabels) {
	for name, value := range src {
		dst[name] = value
	}
}

// ValidateBuilders is a convenience for creating ValidateParams hooks that
// check a list of required builder names.
func ValidateBuilders(names ...string) func(*Registry) error {
	return func(reg *Registry) error {
		for _, n := range names {
			if _, ok := reg.Resolve(n); !ok {
				return fmt.Errorf("initialization failed: missing builder %q", n)
			}
		}
		return nil
	}
}
