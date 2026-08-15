// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSignalSource_ReleasesOwnership(t *testing.T) {
	tests := []struct {
		name       string
		context    func() context.Context
		envelope   func() SignalEnvelope
		params     func() LoopParams
		want       AdmissionOutcome
		wantStatus RunStatus
	}{
		{
			name: "success",
			params: func() LoopParams {
				return signalLoopParams(signalMachineSpec(), successfulSignalBuilder(nil), NoopCheckpoint{})
			},
			want: AdmissionAccepted, wantStatus: StatusSucceeded,
		},
		{
			name: "suspension",
			params: func() LoopParams {
				builder := signalTestBuilder(func(Result) Command {
					return signalTestCommand{name: "work", execute: func() Result {
						return Result{Signal: AwaitApproval}
					}}
				})
				return signalLoopParams(signalMachineSpec(), builder, &InMemoryCheckpoint{})
			},
			want: AdmissionAccepted, wantStatus: StatusSuspended,
		},
		{
			name: "command error",
			params: func() LoopParams {
				builder := signalTestBuilder(func(Result) Command {
					return signalTestCommand{name: "work", execute: func() Result {
						return Result{Err: errors.New("failed")}
					}}
				})
				return signalLoopParams(signalMachineSpec(), builder, NoopCheckpoint{})
			},
			want: AdmissionAccepted, wantStatus: StatusFailed,
		},
		{
			name: "cancelled",
			context: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			params: func() LoopParams {
				return signalLoopParams(signalMachineSpec(), successfulSignalBuilder(nil), NoopCheckpoint{})
			},
			want: AdmissionAccepted, wantStatus: StatusCancelled,
		},
		{
			name: "timeout",
			params: func() LoopParams {
				builder := signalTestBuilder(func(Result) Command {
					return signalTestCommand{name: "work", execute: func() Result {
						time.Sleep(10 * time.Millisecond)
						return Result{Signal: ToolDone}
					}}
				})
				params := signalLoopParams(signalMachineSpec(), builder, NoopCheckpoint{})
				params.CommandTimeout = time.Millisecond
				return params
			},
			want: AdmissionAccepted, wantStatus: StatusFailed,
		},
		{
			name: "checkpoint load failure",
			envelope: func() SignalEnvelope {
				envelope := signalEnvelope("release-checkpoint-load-failure", requestedSignal)
				envelope.Resume = true
				return envelope
			},
			params: func() LoopParams {
				checkpoint := &signalCountCheckpoint{loadErr: errors.New("load failed")}
				return signalLoopParams(signalMachineSpec(), successfulSignalBuilder(nil), checkpoint)
			},
			want: AdmissionRefusedConflict,
		},
		{
			name: "checkpoint save failure",
			params: func() LoopParams {
				checkpoint := &signalCountCheckpoint{
					loadErr: ErrNoCheckpoint, saveErr: errors.New("save failed"),
				}
				return signalLoopParams(signalMachineSpec(), successfulSignalBuilder(nil), checkpoint)
			},
			want: AdmissionAccepted, wantStatus: StatusFailed,
		},
		{
			name: "builder panic",
			params: func() LoopParams {
				builder := signalTestBuilder(func(Result) Command { panic("builder panic") })
				return signalLoopParams(signalMachineSpec(), builder, NoopCheckpoint{})
			},
			want: AdmissionAccepted, wantStatus: StatusFailed,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := NewLoopSignalSource()
			envelope := signalEnvelope("release-"+strings.ReplaceAll(test.name, " ", "-"), requestedSignal)
			if test.envelope != nil {
				envelope = test.envelope()
			}
			ctx := context.Background()
			if test.context != nil {
				ctx = test.context()
			}

			admission := source.Admit(ctx, envelope, test.params())

			require.Equal(t, test.want, admission.Outcome)
			require.Equal(t, test.wantStatus, admission.RunStatus)
			require.False(t, source.Ownership().Held(envelope.RunID))
			release, acquired := source.Ownership().TryAcquire(envelope.RunID)
			require.True(t, acquired, "the next request must acquire immediately")
			release()
		})
	}
}

func TestSignalSource_TraceIsBoundedAndOmitsPayload(t *testing.T) {
	trace := newTopologyTracer()
	params := signalLoopParams(signalMachineSpec(), successfulSignalBuilder(nil), NoopCheckpoint{})
	params.Trace = trace
	envelope := signalEnvelope("trace", requestedSignal)
	envelope.Route = strings.Repeat("r", signalAttributeLimit+20)
	envelope.Payload = json.RawMessage(`{"secret":"never trace me"}`)

	admission := NewLoopSignalSource().Admit(context.Background(), envelope, params)

	require.Equal(t, AdmissionAccepted, admission.Outcome)
	require.NotEmpty(t, *trace.spans)
	sourceSpan := (*trace.spans)[0]
	require.Len(t, fmt.Sprint(sourceSpan.attrs["signal.route"]), signalAttributeLimit)
	for _, span := range *trace.spans {
		require.NotContains(t, fmt.Sprint(span.attrs), "never trace me")
		require.NotContains(t, fmt.Sprint(span.attrs), `"secret"`)
	}
}
