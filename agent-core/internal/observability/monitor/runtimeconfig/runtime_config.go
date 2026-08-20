// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package runtimeconfig compiles trusted runtime declarations into monitor policy.
package runtimeconfig

import (
	"fmt"
	"sort"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

// CompileRecorderConfig compiles a trusted machine, selected tool definitions,
// and run identity into the bounded policy used by a monitor recorder.
func CompileRecorderConfig(
	machine core.MachineSpec,
	defs []catalog.ToolDef,
	runID string,
) (monitor.RecorderConfig, error) {
	workflowValues := machineMetricLabelValues(machine)
	cfg := monitor.RecorderConfig{
		GlobalAttributes: []monitor.AttributePolicy{{
			Name: "agent.name", AllowedValues: []string{machineAgentName(machine)},
		}},
		Envelope: CompileEnvelopePolicy(machine, defs, runID),
	}
	for _, name := range sortedMapKeys(workflowValues) {
		cfg.GlobalAttributes = append(cfg.GlobalAttributes, monitor.AttributePolicy{
			Name: name, AllowedValues: workflowValues[name],
		})
	}
	for _, def := range defs {
		bindings, err := compileToolMetricBindings(def, machine, workflowValues)
		if err != nil {
			return monitor.RecorderConfig{}, err
		}
		cfg.Bindings = append(cfg.Bindings, bindings...)
	}
	return cfg, nil
}

// CompileEnvelopePolicy bounds runtime-owned identity values to the trusted
// machine, selected tools, and current run.
func CompileEnvelopePolicy(
	machine core.MachineSpec,
	defs []catalog.ToolDef,
	runID string,
) monitor.EnvelopePolicy {
	tools := make(map[string]struct{}, len(defs)+len(machine.Transitions))
	for _, def := range defs {
		tools[def.Name] = struct{}{}
	}
	signals := stringSet(machine.Signals.Names())
	for _, transition := range machine.Transitions {
		if transition.Action != "" && transition.Action != "$tool" {
			tools[transition.Action] = struct{}{}
		}
		if transition.Signal != "" {
			signals[transition.Signal] = struct{}{}
		}
	}
	for _, signal := range []core.Signal{
		core.CommandError, core.ToolFailed, core.BudgetExhausted,
	} {
		signals[string(signal)] = struct{}{}
	}
	return monitor.EnvelopePolicy{
		RunID: runID, ToolNames: sortedSetValues(tools),
		States: machine.States.Names(), Signals: sortedSetValues(signals),
	}
}

func compileToolMetricBindings(
	def catalog.ToolDef,
	machine core.MachineSpec,
	workflowValues map[string][]string,
) ([]monitor.MetricBinding, error) {
	if def.Metrics.Disabled {
		return nil, nil
	}
	declared := make(map[string]core.MetricAttribute, len(def.Metrics.Attributes))
	for _, attr := range def.Metrics.Attributes {
		declared[attr.Name] = attr
	}
	bindings := make([]monitor.MetricBinding, 0, len(def.Metrics.Instruments))
	for _, instrument := range def.Metrics.Instruments {
		binding, err := compileToolMetricBinding(def.Name, instrument, declared, machine, workflowValues)
		if err != nil {
			return nil, err
		}
		bindings = append(bindings, binding)
	}
	return bindings, nil
}

func compileToolMetricBinding(
	toolName string,
	instrument core.MetricInstrument,
	declared map[string]core.MetricAttribute,
	machine core.MachineSpec,
	workflowValues map[string][]string,
) (monitor.MetricBinding, error) {
	binding := monitor.MetricBinding{
		ToolName: toolName,
		Schema: monitor.MetricSchema{
			Name: instrument.Name, Kind: monitor.InstrumentKind(instrument.Kind),
			Unit: instrument.Unit, Description: instrument.Description,
		},
	}
	for _, name := range instrument.Attributes {
		attr, ok := declared[name]
		if !ok {
			return monitor.MetricBinding{}, fmt.Errorf(
				"tool %q metric %q attribute %q is not declared", toolName, instrument.Name, name,
			)
		}
		if attr.Redaction == "omit" {
			continue
		}
		values := metricAttributeAllowedValues(toolName, attr, machine, workflowValues)
		if len(values) == 0 {
			return monitor.MetricBinding{}, fmt.Errorf(
				"tool %q metric %q attribute %q has no bounded allowed values",
				toolName, instrument.Name, name,
			)
		}
		binding.Attributes = append(binding.Attributes, monitor.AttributePolicy{
			Name: name, AllowedValues: values,
		})
	}
	return binding, nil
}

func machineMetricLabelValues(machine core.MachineSpec) map[string][]string {
	values := make(map[string]map[string]struct{})
	add := func(labels core.MetricLabels) {
		for name, value := range labels {
			if values[name] == nil {
				values[name] = make(map[string]struct{})
			}
			values[name][value] = struct{}{}
		}
	}
	add(machine.MetricLabels)
	for _, transition := range machine.Transitions {
		add(transition.MetricLabels)
	}
	out := make(map[string][]string, len(values))
	for name, set := range values {
		out[name] = sortedSetValues(set)
	}
	return out
}

func metricAttributeAllowedValues(
	toolName string,
	attr core.MetricAttribute,
	machine core.MachineSpec,
	workflowValues map[string][]string,
) []string {
	if len(attr.AllowedValues) > 0 {
		return append([]string(nil), attr.AllowedValues...)
	}
	switch attr.Source {
	case "tool_name":
		return []string{toolName}
	case "state":
		return machine.States.Names()
	case "signal":
		return machine.Signals.Names()
	case "status":
		return []string{"success", "failure"}
	case "workflow_label":
		return append([]string(nil), workflowValues[attr.Name]...)
	default:
		return nil
	}
}

func machineAgentName(machine core.MachineSpec) string {
	if machine.Name != "" {
		return machine.Name
	}
	return "agent"
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			set[value] = struct{}{}
		}
	}
	return set
}

func sortedSetValues(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		if value != "" {
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func sortedMapKeys(values map[string][]string) []string {
	out := make([]string, 0, len(values))
	for name := range values {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
