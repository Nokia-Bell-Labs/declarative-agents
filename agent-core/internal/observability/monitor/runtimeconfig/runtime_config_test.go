// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package runtimeconfig

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

func TestCompileRecorderConfig_BindsTrustedToolAndWorkflowPolicy(t *testing.T) {
	t.Parallel()
	machine := compilerMachine()
	cfg, err := CompileRecorderConfig(machine, compilerToolDefs(), "run-planner")
	require.NoError(t, err)

	require.Equal(t, monitor.EnvelopePolicy{
		RunID:     "run-planner",
		ToolNames: []string{"read", "run"},
		States:    []string{"Idle", "Working", "Done"},
		Signals: []string{
			"BudgetExhausted", "CommandError", "Seed", "ToolDone", "ToolFailed",
		},
	}, cfg.Envelope)
	require.Equal(t, []monitor.AttributePolicy{
		{Name: "agent.name", AllowedValues: []string{"planner"}},
		{Name: "profile", AllowedValues: []string{"monitor"}},
		{Name: "route_group", AllowedValues: []string{"read", "write"}},
	}, cfg.GlobalAttributes)

	require.Len(t, cfg.Bindings, 1)
	binding := cfg.Bindings[0]
	require.Equal(t, "read", binding.ToolName)
	require.Equal(t, monitor.MetricSchema{
		Name: "filesystem.bytes_read", Kind: monitor.InstrumentHistogram,
		Unit: "By", Description: "Bytes read.",
	}, binding.Schema)
	require.Equal(t, []monitor.AttributePolicy{
		{Name: "operation", AllowedValues: []string{"read"}},
		{Name: "model", AllowedValues: []string{"qwen", "llama"}},
		{Name: "state", AllowedValues: []string{"Idle", "Working", "Done"}},
		{Name: "signal", AllowedValues: []string{"Seed", "ToolDone"}},
		{Name: "status", AllowedValues: []string{"success", "failure"}},
		{Name: "route_group", AllowedValues: []string{"read", "write"}},
	}, binding.Attributes)
}

func TestCompileRecorderConfig_CompiledPolicyFiltersOmittedValues(t *testing.T) {
	t.Parallel()
	cfg, err := CompileRecorderConfig(compilerMachine(), compilerToolDefs(), "run-planner")
	require.NoError(t, err)
	store := monitor.NewStore(monitor.Limits{})
	recorder, err := monitor.NewRecorderWithConfig(store, nil, cfg)
	require.NoError(t, err)

	err = recorder.RecordMetric(context.Background(), monitor.MetricSample{
		Name: "filesystem.bytes_read", Kind: monitor.InstrumentHistogram, Unit: "By",
		ToolName: "read", RunID: "run-planner", State: "Working",
		Signal: "ToolDone", Status: "success", Value: 4,
		Attributes: map[string]string{
			"operation": "read", "profile": "monitor", "route_group": "read",
			"model": "qwen", "secret": "do-not-store",
		},
	})
	require.NoError(t, err)

	snapshot := store.Snapshot()
	require.Equal(t, map[string]string{
		"operation": "read", "profile": "monitor", "route_group": "read", "model": "qwen",
	}, snapshot.RecentSamples[0].Attributes)
	require.Len(t, snapshot.Diagnostics, 1)
	require.NotContains(t, snapshot.Diagnostics[0].Message, "do-not-store")
}

func TestCompileRecorderConfig_RejectsUnboundedAttribute(t *testing.T) {
	t.Parallel()
	defs := []catalog.ToolDef{{
		Name: "invoke",
		Metrics: core.MetricConfig{
			Instruments: []core.MetricInstrument{{
				Name: "llm.tokens", Kind: "histogram", Unit: "1",
				Description: "Token count.", ValueSource: "tokens", Attributes: []string{"model"},
			}},
			Attributes: []core.MetricAttribute{{
				Name: "model", Source: "config_literal", Cardinality: "low", Redaction: "none",
			}},
		},
	}}

	_, err := CompileRecorderConfig(compilerMachine(), defs, "run-planner")

	require.ErrorContains(t, err, `tool "invoke" metric "llm.tokens" attribute "model"`)
	require.ErrorContains(t, err, "has no bounded allowed values")
}

func TestCompileRecorderConfig_RejectsUndeclaredInstrumentAttribute(t *testing.T) {
	t.Parallel()
	defs := []catalog.ToolDef{{
		Name: "read",
		Metrics: core.MetricConfig{Instruments: []core.MetricInstrument{{
			Name: "filesystem.bytes_read", Kind: "histogram", Unit: "By",
			Description: "Bytes read.", ValueSource: "bytes_read", Attributes: []string{"operation"},
		}}},
	}}

	_, err := CompileRecorderConfig(compilerMachine(), defs, "run-planner")

	require.ErrorContains(t, err, `attribute "operation" is not declared`)
}

func TestCompileRecorderConfig_DisabledToolAddsNoBinding(t *testing.T) {
	t.Parallel()
	cfg, err := CompileRecorderConfig(compilerMachine(), []catalog.ToolDef{{
		Name: "read", Metrics: core.MetricConfig{Disabled: true},
	}}, "run-planner")
	require.NoError(t, err)
	require.Empty(t, cfg.Bindings)
	require.Contains(t, cfg.Envelope.ToolNames, "read")
}

func TestCompileRecorderConfig_LegacyMachineUsesAgentIdentity(t *testing.T) {
	t.Parallel()
	cfg, err := CompileRecorderConfig(core.MachineSpec{}, nil, "run-legacy")
	require.NoError(t, err)
	require.Equal(t, monitor.AttributePolicy{
		Name: "agent.name", AllowedValues: []string{"agent"},
	}, cfg.GlobalAttributes[0])
}

func compilerMachine() core.MachineSpec {
	return core.MachineSpec{
		Name:         "planner",
		MetricLabels: core.MetricLabels{"profile": "monitor", "route_group": "read"},
		States: core.StateSpecs{
			{Name: "Idle"}, {Name: "Working"}, {Name: "Done"},
		},
		Signals: core.SignalSpecsFromNames("Seed", "ToolDone"),
		Transitions: []core.TransitionSpec{
			{
				State: "Idle", Signal: "Seed", Next: "Working", Action: "run",
				MetricLabels: core.MetricLabels{"route_group": "write"},
			},
			{State: "Working", Signal: "ToolDone", Next: "Done", Action: "$tool"},
		},
	}
}

func compilerToolDefs() []catalog.ToolDef {
	return []catalog.ToolDef{{
		Name: "read",
		Metrics: core.MetricConfig{
			Instruments: []core.MetricInstrument{{
				Name: "filesystem.bytes_read", Kind: "histogram", Unit: "By",
				Description: "Bytes read.", ValueSource: "bytes_read",
				Attributes: []string{
					"operation", "model", "state", "signal", "status", "route_group", "secret",
				},
			}},
			Attributes: []core.MetricAttribute{
				{Name: "operation", Source: "tool_name", Cardinality: "low"},
				{
					Name: "model", Source: "config_literal", Cardinality: "bounded",
					AllowedValues: []string{"qwen", "llama"},
				},
				{Name: "state", Source: "state", Cardinality: "low"},
				{Name: "signal", Source: "signal", Cardinality: "low"},
				{Name: "status", Source: "status", Cardinality: "low"},
				{Name: "route_group", Source: "workflow_label", Cardinality: "low"},
				{Name: "secret", Source: "config_literal", Cardinality: "low", Redaction: "omit"},
			},
		},
	}}
}
