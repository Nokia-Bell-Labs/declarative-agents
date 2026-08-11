// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel/metric"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolexec "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/exec"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
	toolrest "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
)

type runtimeConfig struct {
	Profile          string
	Machine          string
	Tools            []string
	ToolDeclarations []string
	ToolConfigDirs   []string
	RestDefinitions  []string
	RestConfigDirs   []string
	CoreRoot         string
	Directory        string
	Request          string
	Output           string
	OTelLog          string
	OTelOTLP         string
	OTelMetricOTLP   string
	OTelService      string
	OTelParent       string
	VerboseTrace     bool
	DoltDSN          string
	ResumeCheckpoint string
	ResumeSignal     string
	ChildAgentBinary string
}

type closeableCheckpoint interface {
	core.Checkpoint
	Close() error
}

type openedCheckpoint struct {
	core.Checkpoint
	close func() error
	label string
}

var openDoltCheckpoint = func(dsn, runID string, terminal func(core.State) bool) (closeableCheckpoint, error) {
	return core.OpenDoltCheckpoint(dsn, runID, terminal)
}

func loadRuntimeConfig() (runtimeConfig, error) {
	if flagProfile == "" {
		return runtimeConfig{}, fmt.Errorf("--profile is required")
	}
	p, err := catalog.LoadProfile(flagProfile)
	if err != nil {
		return runtimeConfig{}, fmt.Errorf("load profile: %w", err)
	}
	directory := flagDirectory
	if directory == "" {
		directory = p.Directory
	}
	return runtimeConfig{
		Profile:          canonicalPath(flagProfile),
		Machine:          p.Machine,
		Tools:            append([]string(nil), p.Tools...),
		ToolDeclarations: append([]string(nil), p.ToolDeclarations...),
		ToolConfigDirs:   append([]string(nil), p.ToolConfigDirs...),
		RestDefinitions:  append([]string(nil), p.RestDefinitions...),
		RestConfigDirs:   append([]string(nil), p.RestConfigDirs...),
		CoreRoot:         strings.TrimSpace(flagCoreRoot),
		Directory:        directory,
		Request:          flagRequest,
		Output:           flagOutput,
		OTelLog:          flagOTelLog,
		OTelOTLP:         flagOTelOTLP,
		OTelMetricOTLP:   flagOTelMetricOTLP,
		OTelService:      flagOTelService,
		OTelParent:       flagOTelParent,
		VerboseTrace:     flagVerboseTrace,
		DoltDSN:          flagDoltDSN,
		ResumeCheckpoint: flagResumeCheckpoint,
		ResumeSignal:     flagResumeSignal,
		ChildAgentBinary: flagChildAgent,
	}, nil
}

func loadProfileToolDefs(cfg runtimeConfig) ([]catalog.ToolDef, error) {
	declarations, err := catalog.LoadToolDeclarationsFromDirs(cfg.ToolConfigDirs)
	if err != nil {
		return nil, fmt.Errorf("load tool config dirs: %w", err)
	}
	explicit, err := catalog.LoadToolDeclarations(cfg.ToolDeclarations)
	if err != nil {
		return nil, fmt.Errorf("load tool declarations: %w", err)
	}
	selection, err := catalog.LoadToolSelections(cfg.Tools)
	if err != nil {
		return nil, fmt.Errorf("load tool selection: %w", err)
	}
	defs, err := catalog.SelectTools(catalog.MergeToolDefs(declarations, explicit), selection)
	if err != nil {
		return nil, fmt.Errorf("select tools: %w", err)
	}
	return defs, nil
}

// resolveCheckpoint returns the typed Checkpoint port for the run: the
// Dolt-backed persistent backend when --dolt-dsn is configured, otherwise the
// no-op adapter so a run without persistence keeps disabled-mode behavior
// (srd035-checkpoint-port R5.1, srd036-dolt-state-persistence R1). The "dolt"
// database/sql driver is registered at the composition root (dolt_driver.go),
// which connects to a dolt sql-server over the MySQL wire protocol.
func resolveCheckpoint(cfg runtimeConfig, machine core.MachineSpec, runID string) (openedCheckpoint, error) {
	if cfg.DoltDSN == "" {
		return openedCheckpoint{Checkpoint: core.NoopCheckpoint{}}, nil
	}
	cp, err := openDoltCheckpoint(cfg.DoltDSN, runID, terminalPredicate(machine))
	if err != nil {
		return openedCheckpoint{}, fmt.Errorf("open dolt checkpoint: %w", err)
	}
	return openedCheckpoint{
		Checkpoint: cp,
		close:      cp.Close,
		label:      "loop checkpoint",
	}, nil
}

// resolveRunID returns the stable identity shared by checkpoint, monitor, and
// trace records: the explicit checkpoint id on resume, or a fresh random id.
func resolveRunID(cfg runtimeConfig) (string, error) {
	if id := strings.TrimSpace(cfg.ResumeCheckpoint); id != "" {
		if id == "latest" {
			return "", fmt.Errorf("--resume-checkpoint %q is unsupported; provide an explicit run id", id)
		}
		return id, nil
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("generate run id: %w", err)
	}
	return "run-" + hex.EncodeToString(id[:]), nil
}

// terminalPredicate reports which machine states end a run so the Dolt adapter
// merges the run branch to main (srd036-dolt-state-persistence R4.3).
func terminalPredicate(machine core.MachineSpec) func(core.State) bool {
	terminal := make(map[core.State]bool, len(machine.TerminalStates))
	for _, s := range machine.TerminalStates {
		terminal[core.State(s)] = true
	}
	return func(s core.State) bool { return terminal[s] }
}

type monitorRuntime struct {
	Store    *monitor.Store
	Recorder monitor.RuntimeRecorder
}

func newMonitorRuntime(
	machine core.MachineSpec,
	defs []catalog.ToolDef,
	restDefs toolrest.Collection,
	meter metric.Meter,
	runID string,
) (monitorRuntime, error) {
	if !monitorConfigured(machine, defs, restDefs) {
		return monitorRuntime{}, nil
	}
	store := monitor.NewStore(monitor.Limits{})
	cfg, err := monitorRecorderConfig(machine, defs, runID)
	if err != nil {
		return monitorRuntime{}, err
	}
	recorder, err := monitor.NewRecorderWithConfig(store, meter, cfg)
	if err != nil {
		return monitorRuntime{}, fmt.Errorf("configure monitor recorder: %w", err)
	}
	return monitorRuntime{Store: store, Recorder: recorder}, nil
}

func monitorRecorderConfig(machine core.MachineSpec, defs []catalog.ToolDef, runID string) (monitor.RecorderConfig, error) {
	workflowValues := machineMetricLabelValues(machine)
	cfg := monitor.RecorderConfig{
		GlobalAttributes: []monitor.AttributePolicy{{
			Name: "agent.name", AllowedValues: []string{machineAgentName(machine)},
		}},
		Envelope: monitorEnvelopePolicy(machine, defs, runID),
	}
	for name, values := range workflowValues {
		cfg.GlobalAttributes = append(cfg.GlobalAttributes, monitor.AttributePolicy{Name: name, AllowedValues: values})
	}
	for _, def := range defs {
		bindings, err := toolMetricBindings(def, machine, workflowValues)
		if err != nil {
			return monitor.RecorderConfig{}, err
		}
		cfg.Bindings = append(cfg.Bindings, bindings...)
	}
	return cfg, nil
}

func monitorEnvelopePolicy(machine core.MachineSpec, defs []catalog.ToolDef, runID string) monitor.EnvelopePolicy {
	tools := make(map[string]struct{}, len(defs)+len(machine.Transitions))
	for _, def := range defs {
		tools[def.Name] = struct{}{}
	}
	signals := make(map[string]struct{}, len(machine.Signals)+len(machine.Transitions))
	for _, signal := range machine.Signals.Names() {
		signals[signal] = struct{}{}
	}
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
		RunID:     runID,
		ToolNames: sortedSetValues(tools),
		States:    machine.States.Names(),
		Signals:   sortedSetValues(signals),
	}
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

func toolMetricBindings(
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
		binding, err := toolMetricBinding(def.Name, instrument, declared, machine, workflowValues)
		if err != nil {
			return nil, err
		}
		bindings = append(bindings, binding)
	}
	return bindings, nil
}

func toolMetricBinding(
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
		binding.Attributes = append(binding.Attributes, monitor.AttributePolicy{Name: name, AllowedValues: values})
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
		for value := range set {
			out[name] = append(out[name], value)
		}
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

func monitorState(
	store *monitor.Store,
	recorder monitor.RuntimeRecorder,
	machine *core.MachineSpec,
	defs []catalog.ToolDef,
	commandState core.CommandStateSource,
) toolrest.MonitorState {
	if store == nil {
		return toolrest.MonitorState{}
	}
	return toolrest.MonitorState{
		Store:        store,
		Recorder:     recorder,
		Machine:      machine,
		Tools:        defs,
		CommandState: commandState,
	}
}

func monitorConfigured(machine core.MachineSpec, defs []catalog.ToolDef, restDefs toolrest.Collection) bool {
	if len(machine.MetricLabels) > 0 || transitionsHaveMetricLabels(machine.Transitions) {
		return true
	}
	for _, def := range defs {
		if len(def.Metrics.Instruments) > 0 || len(def.Metrics.Attributes) > 0 || def.Metrics.Disabled {
			return true
		}
	}
	return restDefinitionsHaveMonitorViews(restDefs)
}

func transitionsHaveMetricLabels(transitions []core.TransitionSpec) bool {
	for _, transition := range transitions {
		if len(transition.MetricLabels) > 0 {
			return true
		}
	}
	return false
}

func restDefinitionsHaveMonitorViews(defs toolrest.Collection) bool {
	for _, server := range defs.Servers {
		for _, endpoint := range server.Endpoints {
			if endpoint.MonitorView != "" {
				return true
			}
		}
	}
	return false
}

func commandContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

func selectedBuiltinInits(defs []catalog.ToolDef) map[string]bool {
	return toolregistry.SelectedBuiltinInits(defs)
}

func execBuilder(def catalog.ToolDef, root string) core.Builder {
	return &toolexec.ExecBuilder{Def: def, Root: root}
}
