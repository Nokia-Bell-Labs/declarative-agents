// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/checkpoint"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toollm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/llm"
	toolrest "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
)

func TestRequestSignalSource_ModelFreeProfile(t *testing.T) {
	prepared := prepareRequestSignalFixture(t, "")
	require.NotNil(t, prepared.State.conversation, "a conversation container may exist without a model client")
	require.Empty(t, prepared.State.ensureResolved().Model)
	require.Empty(t, prepared.State.ensureResolved().ProviderName)
	_, modelRegistered := prepared.State.registry.Resolve("invoke_llm")
	require.False(t, modelRegistered)

	runner, _ := requestSignalFixtureRunner(prepared)
	servers, err := launchRequestSignalServers(prepared.State, runner)
	require.NoError(t, err)
	address := servers.addresses["request_signals"]
	require.NotEmpty(t, address)

	body := postRequestSignal(t, address, "start", "model-free-1", "Ready", "hello", "hidden")
	require.Equal(t, "accepted", body["outcome"])
	require.Equal(t, "succeeded", body["run_status"])
	require.Equal(t, "Succeeded", body["state_after"])
	require.NotContains(t, fmt.Sprint(body), "hidden")
	require.NoError(t, servers.Close())

	served := make(chan error, 1)
	go func() { served <- serveRequestSignalSources(prepared) }()
	prepared.Cancel()
	require.NoError(t, <-served)
}

func TestRequestSignalSource_OrdinaryLoopPath(t *testing.T) {
	prepared := prepareRequestSignalFixture(t, "")
	runner, _ := requestSignalFixtureRunner(prepared)

	succeeded := runner.RequestSignal(context.Background(), requestSignalEnvelope(
		"ordinary-success", "StartRequested", "Ready", "normal payload",
	))
	require.Equal(t, core.AdmissionAccepted, succeeded.Outcome)
	require.Equal(t, core.StatusSucceeded, succeeded.RunStatus)
	require.Equal(t, core.State("Succeeded"), succeeded.StateAfter)
	require.Len(t, succeeded.Run.Events, 1)
	require.Equal(t, "echo_request", succeeded.Run.Events[0].CommandName)
	require.Equal(t, core.Signal("WorkCompleted"), succeeded.Run.Events[0].Signal)
	require.JSONEq(t, `{"value":"normal payload"}`, succeeded.Run.Summary)

	failed := runner.RequestSignal(context.Background(), requestSignalEnvelope(
		"ordinary-failure", "FailRequested", "Ready", "domain failure",
	))
	require.Equal(t, core.AdmissionAccepted, failed.Outcome)
	require.Equal(t, core.StatusFailed, failed.RunStatus)
	require.Equal(t, core.State("Failed"), failed.StateAfter)
	require.Len(t, failed.Run.Events, 1)
	require.Equal(t, "fail_request", failed.Run.Events[0].CommandName)
	require.Equal(t, core.Signal("WorkFailed"), failed.Run.Events[0].Signal)

	conflict := runner.RequestSignal(context.Background(), requestSignalEnvelope(
		"ordinary-success", "StartRequested", "Ready", "replay",
	))
	require.Equal(t, core.AdmissionRefusedConflict, conflict.Outcome)
	require.Equal(t, "stale_expected_state", conflict.Stage)
}

func TestRequestSignalSource_SuspendResume(t *testing.T) {
	t.Run("in-memory continuation and noop refusal", func(t *testing.T) {
		prepared := prepareRequestSignalFixture(t, "")
		prepared.State.conversation.Append(modelllm.Message{Role: modelllm.User, Content: "durable"})
		prepared.State.parseRetries = &toollm.ParseErrorRetryTracker{MaxConsecutive: 5}
		prepared.State.parseRetries.ReportParseError()
		runner, store := requestSignalFixtureRunner(prepared)
		runID := "suspend-memory"

		suspended := runner.RequestSignal(context.Background(), requestSignalEnvelope(
			runID, "SuspendRequested", "Ready", "first payload",
		))
		require.Equal(t, core.AdmissionAccepted, suspended.Outcome)
		require.Equal(t, core.StatusSuspended, suspended.RunStatus)
		require.Equal(t, core.State("Suspending"), suspended.StateAfter)
		prepared.State.conversation.Append(modelllm.Message{Role: modelllm.User, Content: "discard"})
		prepared.State.parseRetries.ReportParseError()
		prepared.State.parseRetries.ReportParseError()

		resumed := runner.RequestSignal(context.Background(), requestSignalEnvelope(
			runID, "ContinueRequested", "Suspending", "second payload",
		))
		require.Equal(t, core.AdmissionAccepted, resumed.Outcome)
		require.Equal(t, core.StatusSucceeded, resumed.RunStatus)
		require.Equal(t, core.State("Succeeded"), resumed.StateAfter)
		require.JSONEq(t, `{"value":"second payload"}`, resumed.Run.Summary)
		require.Len(t, resumed.Run.Events, 1)
		require.Equal(t, "echo_request", resumed.Run.Events[0].CommandName)
		require.Equal(t, []modelllm.Message{{Role: modelllm.User, Content: "durable"}},
			prepared.State.conversation.Snapshot())
		require.Equal(t, 1, prepared.State.parseRetries.Snapshot())

		checkpoint, err := store.Open(runID)
		require.NoError(t, err)
		position, execution, err := checkpoint.Load()
		require.NoError(t, err)
		require.Equal(t, core.State("Succeeded"), position.CurrentState)
		require.Len(t, execution, 2)
		require.Equal(t, []string{"suspend", "echo_request"}, []string{
			execution[0].CommandName, execution[1].CommandName,
		})
		require.NotContains(t, fmt.Sprint(execution), "first payload")
		require.Contains(t, fmt.Sprint(execution), "second payload")

		noopRunner := &hostRequestSignalRunner{
			source: core.NewLoopSignalSource(), params: prepared.Params, state: prepared.State,
			checkpoints: func(string) (openedCheckpoint, error) {
				return openedCheckpoint{Checkpoint: core.NoopCheckpoint{}}, nil
			},
		}
		claim := requestSignalEnvelope("suspended-noop", "ContinueRequested", "Suspending", "new")
		claim.Resume = true
		refused := noopRunner.RequestSignal(context.Background(), claim)
		require.Equal(t, core.AdmissionRefusedConflict, refused.Outcome)
		require.Equal(t, "checkpoint_unavailable", refused.Stage)
	})

	t.Run("Dolt-backed continuation", func(t *testing.T) {
		base := startDoltServer(t)
		requireDoltTestDB(t, base)
		prepared := prepareRequestSignalFixture(t, base+doltTestDB)
		runner, _ := requestSignalFixtureRunner(prepared)
		runID := fmt.Sprintf("request-signal-%d", time.Now().UnixNano())

		suspended := runner.RequestSignal(context.Background(), requestSignalEnvelope(
			runID, "SuspendRequested", "Ready", "first",
		))
		require.Equal(t, core.StatusSuspended, suspended.RunStatus)
		resumed := runner.RequestSignal(context.Background(), requestSignalEnvelope(
			runID, "ContinueRequested", "Suspending", "from-dolt",
		))
		require.Equal(t, core.AdmissionAccepted, resumed.Outcome)
		require.Equal(t, core.StatusSucceeded, resumed.RunStatus)
		require.JSONEq(t, `{"value":"from-dolt"}`, resumed.Run.Summary)
	})
}

func TestRequestSignalSource_ModelRunOrResumeRegression(t *testing.T) {
	var calls atomic.Int32
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		require.Equal(t, http.MethodPost, req.Method)
		require.Equal(t, "/api/chat", req.URL.Path)
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"message":{"role":"assistant","content":"mock model answer"},
			"prompt_eval_count":3,
			"eval_count":2
		}`))
	}))
	t.Cleanup(model.Close)

	registry := core.NewRegistry()
	conversation := modelllm.NewConversation(nil, "", modelllm.ChatOptions{})
	def := catalog.ToolDef{
		Name: "invoke_llm", Type: "builtin", Init: "invoke_llm",
		Config: map[string]interface{}{
			"model": "mock-model", "provider": "ollama", "provider_url": model.URL,
			"manifest_state": "Calling", "response_profile": "qwen",
			"system_prompt": "Answer once.", "answer_only": true, "llm_timeout": 2,
		},
	}
	builder, err := toollm.NewInvokeLLMBuilder(def, toollm.InvokeLLMFactoryDeps{
		History: conversation, Registry: registry, Tracer: tracing.NoopTracer{},
		Ctx: context.Background(),
	})
	require.NoError(t, err)
	registry.Register(def.ToToolSpec(), builder)
	params := core.LoopParams{
		InitialState: "Start", InitialSignal: core.Seed, Registry: registry,
		Table: core.TransitionTable{
			{State: "Start", Signal: core.Seed}: {
				NextState: "Calling", Action: func(result core.Result) core.Command {
					return builder.Build(result)
				},
			},
			{State: "Calling", Signal: core.LLMResponded}: {NextState: "Succeeded"},
		},
		IsTerminal: func(state core.State) bool { return state == "Succeeded" },
		Trace:      tracing.NoopTracer{}, CommandTimeout: 2 * time.Second,
		Hooks: core.LoopHooks{
			TerminalStatus: func(core.State) core.RunStatus { return core.StatusSucceeded },
		},
	}

	result, err := runOrResume(runtimeConfig{}, resumeDeps{
		Params: params, State: &agentState{conversation: conversation}, Ctx: context.Background(),
	})

	require.NoError(t, err)
	require.Equal(t, core.StatusSucceeded, result.Status)
	require.Equal(t, core.State("Succeeded"), result.FinalState)
	require.EqualValues(t, 1, calls.Load())
	require.Len(t, conversation.Snapshot(), 2)
	require.Equal(t, "mock model answer", conversation.Snapshot()[1].Content)
}

func prepareRequestSignalFixture(t *testing.T, doltDSN string) preparedRun {
	t.Helper()
	profilePath := profilePathFromTest(t, "request-signal/profile.yaml")
	profile, err := catalog.LoadProfile(profilePath)
	require.NoError(t, err)
	cfg := runtimeConfig{
		Profile: canonicalPath(profilePath), Machine: profile.Machine,
		Tools: profile.Tools, ToolDeclarations: profile.ToolDeclarations,
		ToolConfigDirs: profile.ToolConfigDirs, RestDefinitions: profile.RestDefinitions,
		RestConfigDirs: profile.RestConfigDirs, Directory: profile.Directory,
		Checkpoint: checkpoint.Config{DoltDSN: doltDSN},
	}
	defs, restDefs, err := loadRuntimeDefinitions(cfg)
	require.NoError(t, err)
	machine, err := loadValidatedRuntimeMachine(cfg, defs)
	require.NoError(t, err)
	program, err := buildProgramRef(cfg)
	require.NoError(t, err)
	prepared, err := buildPreparedRun(&cobra.Command{}, runResources{
		Config: cfg, Tracer: tracing.NoopTracer{}, Definitions: defs,
		RestDefinitions: restDefs, Machine: machine, Program: program,
		shutdownTelemetry: func() {},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, prepared.Close()) })
	return prepared
}

func requestSignalFixtureRunner(
	prepared preparedRun,
) (*hostRequestSignalRunner, *requestSignalCheckpointStore) {
	store := &requestSignalCheckpointStore{cfg: prepared.Config, machine: *prepared.Params.MachineSpec}
	return &hostRequestSignalRunner{
		source: core.NewLoopSignalSource(), params: prepared.Params, state: prepared.State,
		checkpoints: store.Open,
	}, store
}

func requestSignalEnvelope(runID, signal, expectedState, data string) core.SignalEnvelope {
	payload, _ := json.Marshal(map[string]string{"data": data, "secret": "hidden"})
	return core.SignalEnvelope{
		Source: "core_request_fixture", Route: "request", RequestID: "request-" + runID,
		RunID: runID, Signal: core.Signal(signal), Payload: payload,
		SensitivePaths: []core.OutputRedactionPath{{"secret"}},
		ExpectedState:  core.State(expectedState),
	}
}

func postRequestSignal(
	t *testing.T,
	address, kind, runID, expectedState, data, secret string,
) map[string]interface{} {
	t.Helper()
	requestBody, err := json.Marshal(map[string]string{
		"kind": kind, "run_id": runID, "expected_state": expectedState,
		"data": data, "secret": secret,
	})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, "http://"+address+"/signals", bytes.NewReader(requestBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "http-"+runID)
	response, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, response.Body.Close()) }()
	require.Equal(t, http.StatusAccepted, response.StatusCode)
	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
	return body
}

var _ toolrest.SignalSourceRunner = (*hostRequestSignalRunner)(nil)
