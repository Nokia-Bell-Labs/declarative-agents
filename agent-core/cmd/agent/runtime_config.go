// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel/metric"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	monitorruntime "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor/runtimeconfig"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/telemetry"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/checkpoint"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolexec "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/exec"
	toollm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/llm"
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
	Telemetry        telemetry.Config
	// CaptureLevel is the composition-root conversion of Telemetry.Capture.
	// observability cannot import tools, so the typed LLM capture level lives here.
	CaptureLevel     toollm.CaptureLevel
	Checkpoint       checkpoint.Config
	DoltConnections  map[string]string
	ChildAgentBinary string
}

func loadRuntimeConfig() (runtimeConfig, error) {
	resolved, err := telemetryCfg.ResolveCapture(telemetryFlags)
	if err != nil {
		return runtimeConfig{}, err
	}
	captureLevel := toollm.CaptureLevel(resolved)
	if flagProfile == "" {
		return runtimeConfig{}, fmt.Errorf("--profile is required")
	}
	p, err := catalog.LoadProfile(flagProfile)
	if err != nil {
		return runtimeConfig{}, fmt.Errorf("load profile: %w", err)
	}
	return runtimeConfigFromProfile(p, captureLevel), nil
}

func runtimeConfigFromProfile(p catalog.AgentProfile, captureLevel toollm.CaptureLevel) runtimeConfig {
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
		Telemetry:        telemetryCfg,
		CaptureLevel:     captureLevel,
		Checkpoint:       checkpointCfg,
		DoltConnections:  doltCfg.Connections,
		ChildAgentBinary: flagChildAgent,
	}
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

// resolveRunID returns the stable identity shared by checkpoint, monitor, and
// trace records: the explicit checkpoint id on resume, or a fresh random id.
func resolveRunID(cfg runtimeConfig) (string, error) {
	id, err := cfg.Checkpoint.ResumeID()
	if err != nil {
		return "", err
	}
	if id != "" {
		return id, nil
	}
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("generate run id: %w", err)
	}
	return "run-" + hex.EncodeToString(buf[:]), nil
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
	cfg, err := monitorruntime.CompileRecorderConfig(machine, defs, runID)
	if err != nil {
		return monitorRuntime{}, err
	}
	recorder, err := monitor.NewRecorderWithConfig(store, meter, cfg)
	if err != nil {
		return monitorRuntime{}, fmt.Errorf("configure monitor recorder: %w", err)
	}
	return monitorRuntime{Store: store, Recorder: recorder}, nil
}

func monitorEnvelopePolicy(machine core.MachineSpec, defs []catalog.ToolDef, runID string) monitor.EnvelopePolicy {
	return monitorruntime.CompileEnvelopePolicy(machine, defs, runID)
}

func monitorState(
	store *monitor.Store,
	recorder monitor.RuntimeRecorder,
	machine *core.MachineSpec,
	defs []catalog.ToolDef,
	commandState core.CommandStateSource,
	declaredMachines ...core.MachineSpec,
) toolrest.MonitorState {
	if store == nil {
		return toolrest.MonitorState{}
	}
	if len(declaredMachines) == 0 && machine != nil {
		declaredMachines = []core.MachineSpec{*machine}
	}
	return toolrest.MonitorState{
		Store:            store,
		Recorder:         recorder,
		Machine:          machine,
		DeclaredMachines: declaredMachines,
		Tools:            defs,
		CommandState:     commandState,
	}
}

func loadDeclaredMonitorMachines(
	store *monitor.Store,
	machine core.MachineSpec,
	cfg runtimeConfig,
	restDefs toolrest.Collection,
) ([]core.MachineSpec, error) {
	if store == nil {
		return nil, nil
	}
	return toolrest.LoadDeclaredMachines(machine, cfg.Machine, filepath.Dir(cfg.Profile), restDefs)
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
