// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
	"github.com/stretchr/testify/require"
)

type recordingSignalSourceRunner struct {
	calls atomic.Int32
	mu    sync.Mutex
	got   []core.SignalEnvelope
	run   func(context.Context, core.SignalEnvelope) core.SignalAdmission
}

func (r *recordingSignalSourceRunner) RequestSignal(
	ctx context.Context,
	envelope core.SignalEnvelope,
) core.SignalAdmission {
	r.calls.Add(1)
	r.mu.Lock()
	r.got = append(r.got, envelope)
	r.mu.Unlock()
	if r.run != nil {
		return r.run(ctx, envelope)
	}
	return core.SignalAdmission{
		Outcome: core.AdmissionAccepted, Source: envelope.Source,
		RequestID: envelope.RequestID, RunID: envelope.RunID, Signal: envelope.Signal,
		StateBefore: "Waiting", StateAfter: "Done", RunStatus: core.StatusSucceeded,
		Stage: "succeeded",
	}
}

func (r *recordingSignalSourceRunner) last() core.SignalEnvelope {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.got[len(r.got)-1]
}

func TestRequestSignalSource_TrustedMapping(t *testing.T) {
	cfg := testSignalSourceBinding()
	request := signalSourceRequestPayload("created", "run-7", "payload", "top-secret")

	t.Run("valid mapping and typed sensitive paths", func(t *testing.T) {
		envelope, mapped, err := mapSignalSourceRequest(cfg, "webhook", "req-1", request)
		require.NoError(t, err)
		require.True(t, mapped)
		require.Equal(t, "orders", envelope.Source)
		require.Equal(t, "webhook", envelope.Route)
		require.Equal(t, "req-1", envelope.RequestID)
		require.Equal(t, "run-7", envelope.RunID)
		require.Equal(t, core.Signal("OrderCreated"), envelope.Signal)
		require.Equal(t, core.State("Waiting"), envelope.ExpectedState)
		require.Equal(t, []core.OutputRedactionPath{{"token"}}, envelope.SensitivePaths)
		require.JSONEq(t, `{"data":"payload","token":"top-secret"}`, string(envelope.Payload))
	})

	t.Run("unknown discriminator is closed and does not call runner", func(t *testing.T) {
		runner := &recordingSignalSourceRunner{}
		state, baseURL := launchSignalSourceTestServer(t, cfg, runner)
		defer stopRESTServer(t, state, "signals")

		body := postJSON(t, baseURL+"/events",
			`{"event":"deleted","run_id":"run-8","expected_state":"Waiting","data":"x","token":"secret"}`,
			http.StatusUnprocessableEntity)

		require.Equal(t, "refused_undeclared", body["outcome"])
		require.Zero(t, runner.calls.Load())
	})

	t.Run("invalid schema does not call runner", func(t *testing.T) {
		runner := &recordingSignalSourceRunner{}
		state, baseURL := launchSignalSourceTestServer(t, cfg, runner)
		defer stopRESTServer(t, state, "signals")

		body := postJSON(t, baseURL+"/events",
			`{"event":"created","run_id":7,"expected_state":"Waiting","data":"x","token":"secret"}`,
			http.StatusBadRequest)

		require.Equal(t, "source_validation_error", body["outcome"])
		require.Zero(t, runner.calls.Load())
	})

	t.Run("raw signal and program authority are refused before runner", func(t *testing.T) {
		for _, authority := range []string{`"signal":"OrderCreated"`, `"machine":"other"`} {
			runner := &recordingSignalSourceRunner{}
			state, baseURL := launchSignalSourceTestServer(t, cfg, runner)
			body := postJSON(t, baseURL+"/events",
				`{"event":"created","run_id":"run-9","expected_state":"Waiting","data":"x","token":"secret",`+authority+`}`,
				http.StatusBadRequest)
			stopRESTServer(t, state, "signals")

			require.Equal(t, "source_validation_error", body["outcome"])
			require.Zero(t, runner.calls.Load())
		}
	})

	t.Run("handler passes only configured envelope", func(t *testing.T) {
		runner := &recordingSignalSourceRunner{}
		state, baseURL := launchSignalSourceTestServer(t, cfg, runner)
		defer stopRESTServer(t, state, "signals")

		_ = postJSON(t, baseURL+"/events",
			`{"event":"created","run_id":"run-10","expected_state":"Waiting","data":"public","token":"secret"}`,
			http.StatusAccepted)

		require.EqualValues(t, 1, runner.calls.Load())
		envelope := runner.last()
		require.Equal(t, []core.OutputRedactionPath{{"token"}}, envelope.SensitivePaths)
		require.NotContains(t, string(envelope.Payload), "event")
		require.NotContains(t, string(envelope.Payload), "expected_state")
		runtime, err := state.runtime("signals")
		require.NoError(t, err)
		require.Empty(t, runtime.queue, "signal_source must not enqueue through emit_signal")
	})
}

func TestSignalSourceDefinition_StrictTypedValidation(t *testing.T) {
	const valid = `
rest:
  version: v1
  servers:
    signals:
      address: 127.0.0.1:0
      endpoints:
        webhook:
          method: POST
          path: /events
          binding: signal_source
          request:
            body_schema:
              type: object
              required: [event, run_id, data, token]
              properties:
                event: {type: string}
                run_id: {type: string}
                data: {type: string}
                token: {type: string}
          signal_source:
            source: orders
            discriminator_field: $.body.event
            signal_mapping: {created: OrderCreated}
            run_id_field: $.body.run_id
            payload:
              data: $.body.data
              token: $.body.token
            sensitive: [token]
            timeout: 1s
            responses:
              accepted: {status: 202}
              refused_undeclared: {status: 422}
              source_validation: {status: 400}
              machine_run_failed: {status: 500}
`
	def, err := ParseDefinition([]byte(valid))
	require.NoError(t, err)
	require.Equal(t, "orders", def.Servers["signals"].Endpoints["webhook"].SignalSource.Source)

	t.Run("unknown config field is rejected", func(t *testing.T) {
		_, err := ParseDefinition([]byte(strings.Replace(
			valid, "            source: orders", "            source: orders\n            profile: caller.yaml", 1)))
		require.ErrorContains(t, err, "profile")
	})

	t.Run("raw signal selector is rejected", func(t *testing.T) {
		def, err := ParseDefinition([]byte(valid))
		require.NoError(t, err)
		endpoint := def.Servers["signals"].Endpoints["webhook"]
		endpoint.Request.BodySchema["properties"].(map[string]interface{})["signal"] =
			map[string]interface{}{"type": "string"}
		endpoint.SignalSource.DiscriminatorField = "$.body.signal"
		server := def.Servers["signals"]
		server.Endpoints["webhook"] = endpoint
		def.Servers["signals"] = server
		require.ErrorContains(t, ValidateDefinition(def), "authority")
	})

	t.Run("sensitive field must be mapped", func(t *testing.T) {
		def, err := ParseDefinition([]byte(valid))
		require.NoError(t, err)
		endpoint := def.Servers["signals"].Endpoints["webhook"]
		endpoint.SignalSource.Sensitive = []string{"not_mapped"}
		server := def.Servers["signals"]
		server.Endpoints["webhook"] = endpoint
		def.Servers["signals"] = server
		require.ErrorContains(t, ValidateDefinition(def), "not mapped payload")
	})

	t.Run("machine request semantics cannot be mixed in", func(t *testing.T) {
		def, err := ParseDefinition([]byte(valid))
		require.NoError(t, err)
		endpoint := def.Servers["signals"].Endpoints["webhook"]
		endpoint.MachineRequest.Timeout = "1s"
		server := def.Servers["signals"]
		server.Endpoints["webhook"] = endpoint
		def.Servers["signals"] = server
		require.ErrorContains(t, ValidateDefinition(def), "must not set machine_request")
	})
}

func testSignalSourceBinding() restdef.SignalSourceBinding {
	return restdef.SignalSourceBinding{
		Source: "orders", DiscriminatorField: "$.body.event",
		SignalMapping: map[string]string{"created": "OrderCreated"},
		RunIDField:    "$.body.run_id", ExpectedStateField: "$.body.expected_state",
		Payload:   map[string]string{"data": "$.body.data", "token": "$.body.token"},
		Sensitive: []string{"token"}, Timeout: "100ms",
		Responses: restdef.SignalSourceResponseMappings{
			Accepted:          restdef.SignalSourceResponse{Status: http.StatusAccepted},
			RefusedUndeclared: restdef.SignalSourceResponse{Status: http.StatusUnprocessableEntity},
			SourceValidation:  restdef.SignalSourceResponse{Status: http.StatusBadRequest},
			MachineRunFailed:  restdef.SignalSourceResponse{Status: http.StatusInternalServerError},
		},
	}
}

func signalSourceRequestPayload(
	event string,
	runID string,
	data string,
	token string,
) map[string]interface{} {
	body := map[string]interface{}{
		"event": event, "run_id": runID, "expected_state": "Waiting",
		"data": data, "token": token,
	}
	return map[string]interface{}{"body": body}
}

func launchSignalSourceTestServer(
	t *testing.T,
	cfg restdef.SignalSourceBinding,
	runner SignalSourceRunner,
) (*ServerState, string) {
	t.Helper()
	server := restdef.Server{
		Address: "127.0.0.1:0", Shutdown: restdef.ShutdownConfig{Timeout: "200ms"},
		Endpoints: map[string]restdef.Endpoint{"webhook": {
			Method: http.MethodPost, Path: "/events", Binding: bindingSignalSource,
			Request: restdef.RequestBinding{BodySchema: map[string]interface{}{
				"type": "object",
				"required": []interface{}{
					"event", "run_id", "expected_state", "data", "token",
				},
				"properties": map[string]interface{}{
					"event":          map[string]interface{}{"type": "string"},
					"run_id":         map[string]interface{}{"type": "string"},
					"expected_state": map[string]interface{}{"type": "string"},
					"data":           map[string]interface{}{"type": "string"},
					"token":          map[string]interface{}{"type": "string"},
				},
			}},
			SignalSource: cfg,
		}},
	}
	require.NoError(t, ValidateDefinition(restdef.Definition{
		Version: "v1", Servers: map[string]restdef.Server{"signals": server},
	}))
	state := NewServerState()
	_, baseURL := launchRESTServerDefinition(t, state, ServerDefinition{
		Name: "signals", Server: server, SignalSourceRunner: runner,
	})
	return state, baseURL
}

func decodeSignalSourceBody(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(raw), &body))
	return body
}
