// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/telemetry"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
	"go.opentelemetry.io/otel"
)

func machineRequestRunner(runner MachineRequestRunner) MachineRequestRunner {
	if runner != nil {
		return runner
	}
	return defaultMachineRequestRunner{}
}

type defaultMachineRequestRunner struct{}

func (defaultMachineRequestRunner) RunMachineRequest(
	ctx context.Context,
	req MachineRequestRun,
) (MachineRequestResult, error) {
	if req.Config.MachineSpec == nil {
		return MachineRequestResult{}, fmt.Errorf("machine_config_invalid: machine_request machine spec is not configured")
	}
	var last core.Result
	rr, err := core.Loop(machineRequestLoopParams(ctx, req, &last), ctx)
	if err != nil {
		return MachineRequestResult{}, fmt.Errorf("machine_config_invalid: %w", err)
	}
	if rr.Status == core.StatusCancelled {
		return MachineRequestResult{}, fmt.Errorf("machine_timeout: request machine timed out")
	}
	return machineRequestResult(req, rr, last)
}

func machineRequestLoopParams(ctx context.Context, req MachineRequestRun, last *core.Result) core.LoopParams {
	initialSignal := machineRequestInitialSignal(req.Config)
	seed := requestSeed(req, initialSignal)
	return core.LoopParams{
		MachineSpec: req.Config.MachineSpec, Registry: req.Config.Registry,
		InitFunc: req.Config.InitFunc, ToolAction: req.Config.ToolAction,
		InitialSignal: initialSignal, InitialResult: seed,
		InitialExecution: machineRequestSeedExecution(seed, nil),
		Budget:           req.Config.Budget,
		CommandTimeout:   parseDuration(req.Config.CommandTimeout, 0),
		Trace:            requestScopedTrace(ctx),
		RunID:            req.RunID, RequestID: req.RequestID,
		ConversationID: req.ConversationID, AgentName: machineRequestAgentName(req),
		Directory: ".", MonitorRecorder: machineRequestMonitorRecorder(req),
		Hooks: core.LoopHooks{
			TerminalStatus: machineRequestTerminalStatus(req.Config),
			OnResult: func(rr core.RunResult, res core.Result) core.RunResult {
				*last = res
				return rr
			},
		},
	}
}

func machineRequestMonitorRecorder(req MachineRequestRun) monitor.RuntimeRecorder {
	scoped, ok := req.MonitorRecorder.(monitor.TrustedEnvelopeRecorder)
	if !ok || req.Config.MachineSpec == nil {
		return req.MonitorRecorder
	}
	return scoped.WithTrustedEnvelope(machineRequestEnvelopePolicy(req.Config, req.RunID))
}

func machineRequestEnvelopePolicy(cfg restdef.MachineRequest, runID string) monitor.EnvelopePolicy {
	machine := cfg.MachineSpec
	tools := make(map[string]struct{}, len(machine.Transitions))
	states := stringValueSet(machine.States.Names()...)
	signals := stringValueSet(machine.Signals.Names()...)
	for _, state := range machine.TerminalStates {
		states[state] = struct{}{}
		signals[state] = struct{}{}
	}
	collectTransitionEnvelope(machine.Transitions, tools, states, signals)
	collectRequestSignals(cfg, signals)
	return monitor.EnvelopePolicy{
		RunID: runID, ToolNames: sortedStringSet(tools),
		States: sortedStringSet(states), Signals: sortedStringSet(signals),
	}
}

func collectTransitionEnvelope(
	transitions []core.TransitionSpec,
	tools, states, signals map[string]struct{},
) {
	for _, transition := range transitions {
		states[transition.State] = struct{}{}
		states[transition.Next] = struct{}{}
		signals[transition.Signal] = struct{}{}
		if transition.Action != "" && transition.Action != "$tool" {
			tools[transition.Action] = struct{}{}
		}
		if transition.ForEach != nil {
			collectForEachEnvelope(*transition.ForEach, tools, signals)
		}
	}
}

func collectForEachEnvelope(spec core.ForEachSpec, tools, signals map[string]struct{}) {
	tools["for_each.join"] = struct{}{}
	for _, signal := range spec.ContinueOn {
		signals[signal] = struct{}{}
	}
	for _, signal := range spec.AbortOn {
		signals[signal] = struct{}{}
	}
	for _, signal := range []string{
		spec.Join.Signals.AllSuccess,
		spec.Join.Signals.Partial,
		spec.Join.Signals.Failed,
		spec.Join.Signals.Empty,
	} {
		signals[signal] = struct{}{}
	}
}

func collectRequestSignals(cfg restdef.MachineRequest, signals map[string]struct{}) {
	for signal := range cfg.Response.TerminalSignals {
		signals[signal] = struct{}{}
	}
	for signal := range cfg.Response.TerminalStates {
		signals[signal] = struct{}{}
	}
	signals[string(machineRequestInitialSignal(cfg))] = struct{}{}
	for _, signal := range []core.Signal{
		core.CommandError, core.ToolFailed, core.BudgetExhausted,
	} {
		signals[string(signal)] = struct{}{}
	}
}

func stringValueSet(values ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			set[value] = struct{}{}
		}
	}
	return set
}

func sortedStringSet(set map[string]struct{}) []string {
	values := make([]string, 0, len(set))
	for value := range set {
		if value != "" {
			values = append(values, value)
		}
	}
	sort.Strings(values)
	return values
}

// requestScopedTrace wraps the process tracer provider rooted at the
// machine_request span in ctx, so request-scoped spans join the connected trace.
// When the global provider is the no-op default (telemetry not initialized), the
// wrapped tracer is a no-op and dispatch behaves exactly as it did under NoopTracer.
func requestScopedTrace(ctx context.Context) tracing.Tracer {
	return telemetry.TraceAdapter{T: telemetry.NewTraceFromProvider(otel.GetTracerProvider(), "agent-core/machine_request", ctx)}
}

func machineRequestAgentName(req MachineRequestRun) string {
	if req.RequestID != "" {
		return "machine_request:" + req.RequestID
	}
	if req.Server != "" && req.Route != "" {
		return "machine_request:" + req.Server + "/" + req.Route
	}
	return "machine_request"
}

// machineRequestTerminalStatus uses declared machine outcome policy first.
// HTTP response mappings remain a compatibility fallback for older machines.
//
// The terminal-state map answers this directly. The terminal-signal map is
// consulted next only for the machines that name a state after the signal
// reaching it, where the two keys coincide; that lookup predates the state map
// and stays so those configurations keep their classification (GH-615).
func machineRequestTerminalStatus(cfg restdef.MachineRequest) func(core.State) core.RunStatus {
	return func(state core.State) core.RunStatus {
		if status, ok := core.DeclaredTerminalStatus(cfg.MachineSpec, state); ok {
			return status
		}
		if mapping, ok := cfg.Response.TerminalStates[string(state)]; ok {
			return runStatusForHTTP(mapping.Status)
		}
		if mapping, ok := cfg.Response.TerminalSignals[string(state)]; ok {
			return runStatusForHTTP(mapping.Status)
		}
		return core.TerminalStatusForState(cfg.MachineSpec, state)
	}
}

// runStatusForHTTP reads a mapped status code as run success or failure. An
// unset status defaults to 200 at write time, so it reads as success here.
func runStatusForHTTP(status int) core.RunStatus {
	if status == 0 || (status >= 200 && status < 400) {
		return core.StatusSucceeded
	}
	return core.StatusFailed
}

func machineRequestInitialSignal(cfg restdef.MachineRequest) core.Signal {
	if cfg.InitialSignal == "" {
		return core.Seed
	}
	return core.Signal(cfg.InitialSignal)
}

// requestSeed builds the Result that seeds a machine_request run's first word.
// The mapped request input is exposed under a "parameters" key, the key both
// filesystem words (extractStringParam) and REST-client words (runtimeParams)
// read, so either can be the first word. The transport authority (method,
// server, route) is omitted: a request-machine word must not see it, and a
// top-level "method" would trip the REST runtime-input authority guard. The URL
// path and request id are kept because they name no transport authority and let
// adapters derive request context (srd030 R2, R3.2).
func requestSeed(req MachineRequestRun, signal core.Signal) core.Result {
	payload := req.Payload
	if payload == nil {
		payload = map[string]interface{}{}
	}
	seed := map[string]interface{}{"parameters": payload}
	if req.Path != "" {
		seed["path"] = req.Path
	}
	if req.RequestID != "" {
		seed["request_id"] = req.RequestID
	}
	data, err := json.Marshal(seed)
	if err != nil {
		return core.Result{
			Signal: signal, Output: `{"parameters":{}}`,
			Redaction: machineRequestSeedRedaction(req.Config.Request),
		}
	}
	return core.Result{
		Signal: signal, Output: string(data),
		Redaction: machineRequestSeedRedaction(req.Config.Request),
	}
}

func machineRequestSeedRedaction(mapping restdef.MachineRequestMapping) core.OutputRedaction {
	paths := make([]core.OutputRedactionPath, 0, len(mapping.Sensitive))
	for _, name := range mapping.Sensitive {
		paths = append(paths, core.OutputRedactionPath{"parameters", name})
	}
	return core.OutputRedaction{
		Version: core.OutputRedactionVersion1,
		Paths:   paths,
	}
}

// machineRequestSeedExecution makes trusted request input addressable after
// intervening words as $from(seed).parameters.<field>. Existing execution wins
// on resume so the synthetic forward entry is never duplicated. DigestResult
// applies the same output-redaction boundary as loop dispatch (srd030 R3.8,
// srd038 R5).
func machineRequestSeedExecution(seed core.Result, existing core.Execution) core.Execution {
	if len(existing) > 0 {
		return existing
	}
	return core.Execution{{
		CommandName: "seed",
		Label:       "seed",
		Signal:      seed.Signal,
		Result:      core.DigestResult(seed),
	}}
}

func machineRequestResult(req MachineRequestRun, rr core.RunResult, last core.Result) (MachineRequestResult, error) {
	if last.Signal == "" {
		return MachineRequestResult{}, fmt.Errorf("response_missing: request machine produced no response signal")
	}
	output := map[string]interface{}{}
	if last.Output != "" {
		if err := json.Unmarshal([]byte(last.Output), &output); err != nil {
			// A terminal word that emits plain text rather than a JSON object (for
			// example invoke_llm, whose output is the raw model answer) is wrapped
			// under "output" so a response body selector can map $.output. JSON
			// object outputs unmarshal above unchanged (srd030 R4.3).
			output = map[string]interface{}{"output": last.Output}
		}
	}
	return MachineRequestResult{
		Server: req.Server, Route: req.Route, Machine: req.Config.MachineSpec.Name,
		TerminalSignal: string(last.Signal), Output: output, Run: rr,
	}, nil
}
