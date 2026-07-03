// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel/metric"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolexec "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/exec"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
	toolrest "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
)

const defaultStateStoreDirName = ".agent-state"

type runtimeConfig struct {
	Machine          string
	Tools            []string
	ToolDeclarations []string
	ToolConfigDirs   []string
	RestDefinitions  []string
	RestConfigDirs   []string
	Directory        string
	Request          string
	Output           string
	OTelLog          string
	OTelParent       string
	VerboseTrace     bool
	StateStoreDir    string
	DoltDSN          string
	ResumeCheckpoint string
	ResumeSignal     string
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
		Machine:          p.Machine,
		Tools:            append([]string(nil), p.Tools...),
		ToolDeclarations: append([]string(nil), p.ToolDeclarations...),
		ToolConfigDirs:   append([]string(nil), p.ToolConfigDirs...),
		RestDefinitions:  append([]string(nil), p.RestDefinitions...),
		RestConfigDirs:   append([]string(nil), p.RestConfigDirs...),
		Directory:        directory,
		Request:          flagRequest,
		Output:           flagOutput,
		OTelLog:          flagOTelLog,
		OTelParent:       flagOTelParent,
		VerboseTrace:     flagVerboseTrace,
		StateStoreDir:    flagStateStoreDir,
		DoltDSN:          flagDoltDSN,
		ResumeCheckpoint: flagResumeCheckpoint,
		ResumeSignal:     flagResumeSignal,
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

func resolveStateStore(cfg runtimeConfig) core.StateStore {
	root := resolveStateStoreRoot(cfg)
	if root == "" {
		return nil
	}
	return core.NewFileStore(root)
}

func resolveStateStoreRoot(cfg runtimeConfig) string {
	if cfg.StateStoreDir != "" {
		return cfg.StateStoreDir
	}
	if cfg.Directory != "" {
		return filepath.Join(cfg.Directory, defaultStateStoreDirName)
	}
	return ""
}

// resolveCheckpoint returns the typed Checkpoint port for the run: the
// Dolt-backed persistent backend when a DSN/repo path is configured, otherwise
// the no-op adapter so a run without persistence keeps disabled-mode behavior
// (srd035-checkpoint-port R5.1, srd036-dolt-state-persistence R1). The Dolt
// driver is registered at the composition root via a blank import (#37b); until
// then a --dolt-dsn run surfaces the unregistered-driver error here.
func resolveCheckpoint(cfg runtimeConfig, machine core.MachineSpec) (core.Checkpoint, error) {
	if cfg.DoltDSN == "" {
		return core.NoopCheckpoint{}, nil
	}
	cp, err := core.OpenDoltCheckpoint(cfg.DoltDSN, resolveRunID(cfg), terminalPredicate(machine))
	if err != nil {
		return nil, fmt.Errorf("open dolt checkpoint: %w", err)
	}
	return cp, nil
}

// resolveRunID names the Dolt run branch: the explicit --resume-checkpoint id
// when resuming a known run, otherwise a fresh timestamp-based id.
func resolveRunID(cfg runtimeConfig) string {
	if id := cfg.ResumeCheckpoint; id != "" && id != "latest" {
		return id
	}
	return fmt.Sprintf("run-%d", time.Now().UTC().UnixNano())
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
) monitorRuntime {
	if !monitorConfigured(machine, defs, restDefs) {
		return monitorRuntime{}
	}
	store := monitor.NewStore(monitor.Limits{})
	return monitorRuntime{Store: store, Recorder: monitor.NewRecorder(store, meter)}
}

func monitorState(store *monitor.Store, machine *core.MachineSpec, defs []catalog.ToolDef) toolrest.MonitorState {
	if store == nil {
		return toolrest.MonitorState{}
	}
	return toolrest.MonitorState{Store: store, Machine: machine, Tools: defs}
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
