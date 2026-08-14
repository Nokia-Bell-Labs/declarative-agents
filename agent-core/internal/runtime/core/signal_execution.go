// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
)

const (
	signalAttributeLimit   = 128
	identityAttributeLimit = 256
)

// Admit evaluates the envelope without probing StateMachine.Step, then gives an
// accepted envelope to the ordinary Loop. Ownership is always released by the
// deferred boundary, including panics from initialization or command builders.
func (s *LoopSignalSource) Admit(
	ctx context.Context,
	envelope SignalEnvelope,
	params LoopParams,
) (admission SignalAdmission) {
	start := time.Now()
	tr := signalTracer(params.Trace)
	params.Trace = tr
	sourceTrace, traceDone := tr.Push("admit_signal", signalTraceStartAttrs(envelope)...)
	admission = SignalAdmission{
		Source: envelope.Source, RequestID: envelope.RequestID, RunID: envelope.RunID,
		Signal: envelope.Signal, Stage: "admission_started",
	}
	var release func()
	defer func() {
		s.finishAdmission(
			&admission, envelope, release, recover(),
			start, sourceTrace, traceDone,
		)
	}()
	s.admitSignal(ctx, envelope, params, &admission, &release)
	return admission
}

func signalTracer(tr tracing.Tracer) tracing.Tracer {
	if tr == nil {
		return tracing.NoopTracer{}
	}
	return tr
}

func (s *LoopSignalSource) finishAdmission(
	admission *SignalAdmission,
	envelope SignalEnvelope,
	release func(),
	recovered any,
	start time.Time,
	tr tracing.Tracer,
	traceDone func(),
) {
	if recovered != nil {
		admission.Err = fmt.Errorf("signal source panic: %v", recovered)
		admission.Stage = "panic"
		if admission.Accepted() {
			admission.Run = RunResult{
				Status: StatusFailed, FinalState: admission.StateBefore,
				LastError: admission.Err,
			}
			admission.RunStatus = StatusFailed
			admission.StateAfter = admission.StateBefore
			s.remember(envelope.RunID, admission.Run, admission.StateBefore)
		} else {
			admission.Outcome = AdmissionRefusedConflict
		}
	}
	if release != nil {
		release()
	}
	admission.Elapsed = time.Since(start)
	finishSignalTrace(tr, *admission)
	traceDone()
}

func (s *LoopSignalSource) admitSignal(
	ctx context.Context,
	envelope SignalEnvelope,
	params LoopParams,
	admission *SignalAdmission,
	release *func(),
) {
	spec, err := admissionSpec(params)
	if err != nil {
		admission.Outcome = AdmissionRefusedConflict
		admission.Stage = "machine_load_failed"
		admission.Err = err
		return
	}
	admission.StateBefore = initialAdmissionState(params, spec)
	admission.StateAfter = admission.StateBefore
	if !signalDeclared(spec, envelope.Signal) {
		admission.Outcome = AdmissionRefusedUndeclared
		admission.Stage = "undeclared"
		return
	}
	ownedRelease, acquired := s.ownership.TryAcquire(envelope.RunID)
	if !acquired {
		admission.Outcome = AdmissionRefusedConflict
		admission.Stage = "concurrent_conflict"
		return
	}
	*release = ownedRelease
	s.admitOwnedSignal(ctx, envelope, params, spec, admission)
}

func (s *LoopSignalSource) admitOwnedSignal(
	ctx context.Context,
	envelope SignalEnvelope,
	params LoopParams,
	spec MachineSpec,
	admission *SignalAdmission,
) {
	params, current, stage, err := s.paramsAtCurrentPosition(envelope, params, spec)
	admission.StateBefore = current
	admission.StateAfter = current
	if err != nil {
		admission.Outcome = AdmissionRefusedConflict
		admission.Stage = stage
		admission.Err = err
		return
	}
	if envelope.ExpectedState != "" && envelope.ExpectedState != current {
		admission.Outcome = AdmissionRefusedConflict
		admission.Stage = "stale_expected_state"
		return
	}
	if exactTransitionCount(spec, current, envelope.Signal) != 1 {
		admission.Outcome = AdmissionRefusedConflict
		admission.Stage = "no_exact_transition"
		return
	}
	params = acceptedSignalParams(params, spec, current, envelope)
	s.executeAcceptedSignal(ctx, envelope, params, admission)
}

func acceptedSignalParams(
	params LoopParams,
	spec MachineSpec,
	current State,
	envelope SignalEnvelope,
) LoopParams {
	params.MachineSpec = &spec
	params.InitialState = current
	params.InitialSignal = envelope.Signal
	params.InitialResult = signalEnvelopeResult(envelope)
	params.PreserveInitialResultOutput = true
	params.RunID = envelope.RunID
	params.RequestID = envelope.RequestID
	return params
}

func (s *LoopSignalSource) executeAcceptedSignal(
	ctx context.Context,
	envelope SignalEnvelope,
	params LoopParams,
	admission *SignalAdmission,
) {
	admission.Outcome = AdmissionAccepted
	admission.Stage = "accepted"
	run, runErr := Loop(params, ctx)
	admission.Run = run
	admission.Err = runErr
	admission.RunStatus = run.Status
	admission.StateAfter = run.FinalState
	if admission.StateAfter == "" {
		admission.StateAfter = admission.StateBefore
	}
	admission.Stage = signalRunStage(run, runErr)
	s.remember(envelope.RunID, run, admission.StateAfter)
}

func signalEnvelopeResult(envelope SignalEnvelope) Result {
	raw := Result{
		Output: string(envelope.Payload),
		Signal: envelope.Signal,
		Redaction: OutputRedaction{
			Version: OutputRedactionVersion1,
			Paths:   cloneOutputRedactionPaths(envelope.SensitivePaths),
		},
	}
	digest := DigestResult(raw)
	redaction := raw.Redaction
	if digest.RedactionStatus != OutputRedactionOmitted {
		redaction = OutputRedaction{
			Version: digest.RedactionVersion,
			Paths:   cloneOutputRedactionPaths(digest.RedactedPaths),
		}
	}
	return Result{
		Output: digest.Output, Signal: envelope.Signal, Redaction: redaction,
	}
}

func signalRunStage(run RunResult, err error) string {
	if run.Status == StatusCancelled {
		return "cancelled"
	}
	if errors.Is(err, ErrCheckpointSaveFailed) ||
		errors.Is(run.LastError, ErrCheckpointSaveFailed) ||
		errors.Is(run.LastError, ErrConversationSnapshotFailed) ||
		errors.Is(run.LastError, ErrDomainSnapshotFailed) {
		return "checkpoint_save_failed"
	}
	if run.Status == StatusSuspended {
		return "suspended"
	}
	if run.Status == StatusSucceeded {
		return "succeeded"
	}
	if run.Status == StatusBudgetExceeded {
		return "budget_exceeded"
	}
	if run.LastError != nil {
		if strings.Contains(run.LastError.Error(), "timeout executing") {
			return "timeout"
		}
		return "command_error"
	}
	return "run_failed"
}

func signalTraceStartAttrs(envelope SignalEnvelope) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("signal.source", boundedSignalAttribute(envelope.Source, signalAttributeLimit)),
		attribute.String("signal.route", boundedSignalAttribute(envelope.Route, signalAttributeLimit)),
		AttrRequestID.String(boundedSignalAttribute(envelope.RequestID, identityAttributeLimit)),
		attribute.String("run.id", boundedSignalAttribute(envelope.RunID, identityAttributeLimit)),
		attribute.String("signal.name", boundedSignalAttribute(string(envelope.Signal), signalAttributeLimit)),
	}
}

func finishSignalTrace(tr tracing.Tracer, admission SignalAdmission) {
	attrs := []attribute.KeyValue{
		attribute.String("signal.admission.outcome", string(admission.Outcome)),
		attribute.String("signal.stage", admission.Stage),
		attribute.String("signal.state_before", boundedSignalAttribute(string(admission.StateBefore), signalAttributeLimit)),
		attribute.String("signal.state_after", boundedSignalAttribute(string(admission.StateAfter), signalAttributeLimit)),
		attribute.String("run.status", string(admission.RunStatus)),
		attribute.Int64("signal.elapsed_ms", admission.Elapsed.Milliseconds()),
	}
	if admission.Err != nil {
		attrs = append(attrs, attribute.String(
			"error.type", boundedSignalAttribute(fmt.Sprintf("%T", admission.Err), signalAttributeLimit),
		))
	}
	tr.SetAttributes(attrs...)
	tr.Event("signal_source.stage", attrs...)
}

func boundedSignalAttribute(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
