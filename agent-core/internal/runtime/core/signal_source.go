// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package core

import (
	"context"
	"encoding/json"
	"time"
)

// SignalEnvelope is the transport-neutral result of validating and mapping one
// source request. Payload is structured JSON; SensitivePaths are trusted,
// typed paths selected by source configuration rather than by the caller.
type SignalEnvelope struct {
	Source         string                `json:"source"`
	Route          string                `json:"route,omitempty"`
	RequestID      string                `json:"request_id"`
	RunID          string                `json:"run_id"`
	Signal         Signal                `json:"signal"`
	Payload        json.RawMessage       `json:"payload"`
	SensitivePaths []OutputRedactionPath `json:"sensitive_paths,omitempty"`
	ExpectedState  State                 `json:"expected_state,omitempty"`
	// Resume claims that RunID identifies a suspended persisted run. It prevents
	// a missing or disabled checkpoint from silently becoming a fresh run.
	Resume bool `json:"resume,omitempty"`
}

// AdmissionOutcome classifies the lookup-only decision made before the loop can
// step the machine or construct a command.
type AdmissionOutcome string

const (
	AdmissionAccepted          AdmissionOutcome = "accepted"
	AdmissionRefusedUndeclared AdmissionOutcome = "refused_undeclared"
	AdmissionRefusedConflict   AdmissionOutcome = "refused_conflict"
)

// SignalAdmission reports admission separately from the subsequent run. An
// accepted outcome remains accepted when command execution or persistence later
// fails; Err and RunStatus describe that later failure.
type SignalAdmission struct {
	Outcome     AdmissionOutcome `json:"outcome"`
	Source      string           `json:"source"`
	RequestID   string           `json:"request_id"`
	RunID       string           `json:"run_id"`
	Signal      Signal           `json:"signal"`
	StateBefore State            `json:"state_before"`
	StateAfter  State            `json:"state_after"`
	RunStatus   RunStatus        `json:"run_status,omitempty"`
	Stage       string           `json:"stage,omitempty"`
	Elapsed     time.Duration    `json:"-"`
	Run         RunResult        `json:"run,omitempty"`
	Err         error            `json:"-"`
}

// Accepted reports whether dispatch ownership and one exact machine transition
// were admitted, independently of the eventual run status.
func (a SignalAdmission) Accepted() bool {
	return a.Outcome == AdmissionAccepted
}

// SignalSource admits validated envelopes into an ordinary core Loop. The
// caller supplies profile-owned LoopParams, including the MachineSpec,
// Registry initialization, checkpoint, and limits for this run.
type SignalSource interface {
	Admit(context.Context, SignalEnvelope, LoopParams) SignalAdmission
}

var _ SignalSource = (*LoopSignalSource)(nil)
