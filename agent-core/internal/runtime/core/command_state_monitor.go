// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package core

import (
	"sync"
	"time"
)

// MonitorCommandStateEntry is the monitor-facing projection of one labeled step:
// the redaction-cleared declared output plus the run envelope recorded when the
// step ran. It never carries a receipt, so a monitor reader cannot reach
// transport authority (srd038-command-state-store R3.3, srd033-monitor-rest-api
// R7.2).
type MonitorCommandStateEntry struct {
	// Available reports whether Output crossed the srd038 redaction boundary.
	// A matched step whose output is omitted, unversioned, or unknown-version is
	// present with Available false and an empty Output (srd033 R7.4).
	Available bool
	Output    string
	State     string
	Signal    string
	Iteration int
	UpdatedAt time.Time
}

// CommandStateSource yields the latest recorded step for a label over the
// current run. The monitor command_state view reads it. A live source is
// refreshed per dispatch so a background monitor server observes progress rather
// than a launch-time snapshot frozen at the first iteration (srd033 R7.1).
type CommandStateSource interface {
	// LookupCommandState returns the latest recorded step whose authored label or
	// executed command name matches label, newest first. found is false when no
	// step matches, which the view renders as an explicit absent entry
	// (srd033 R7.3).
	LookupCommandState(label string) (entry MonitorCommandStateEntry, found bool)
}

// CommandStateObserver receives the run's execution log after each dispatch so a
// live command-state source can refresh. The loop drives it, mirroring the way
// MonitorRecorderAware receives the tool-facing recorder (srd033 R7.1).
type CommandStateObserver interface {
	ObserveCommandState(execution Execution)
}

// LiveCommandStateSource is the shared handle between the loop, which refreshes
// it per dispatch through ObserveCommandState, and the monitor REST server,
// which reads it through LookupCommandState. It holds a receipt-free copy of the
// run's execution log behind a read-write lock.
type LiveCommandStateSource struct {
	mu        sync.RWMutex
	execution Execution
}

// NewLiveCommandStateSource returns an empty live source ready to be wired into
// LoopParams.CommandStateObserver and MonitorState.CommandState.
func NewLiveCommandStateSource() *LiveCommandStateSource {
	return &LiveCommandStateSource{}
}

// ObserveCommandState replaces the retained execution with a clone of the loop's
// current log. Cloning keeps the loop's slice private to the loop.
func (s *LiveCommandStateSource) ObserveCommandState(execution Execution) {
	clone := CloneExecution(execution)
	s.mu.Lock()
	s.execution = clone
	s.mu.Unlock()
}

// LookupCommandState scans the retained execution newest first and matches
// either the authored label or the executed command name, so duplicate labels
// and label/command-name collisions resolve to the highest step index, the same
// rule the selector view uses (srd038 R2.2).
func (s *LiveCommandStateSource) LookupCommandState(label string) (MonitorCommandStateEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := len(s.execution) - 1; i >= 0; i-- {
		e := s.execution[i]
		if (e.Label != "" && e.Label == label) || e.CommandName == label {
			output, available := monitorResolveEntryOutput(e.Result)
			return MonitorCommandStateEntry{
				Available: available,
				Output:    output,
				State:     string(e.ToState),
				Signal:    string(e.Signal),
				Iteration: e.Iteration,
				UpdatedAt: e.Timestamp,
			}, true
		}
	}
	return MonitorCommandStateEntry{}, false
}

var _ CommandStateSource = (*LiveCommandStateSource)(nil)
var _ CommandStateObserver = (*LiveCommandStateSource)(nil)

// monitorResolveEntryOutput mirrors the redaction gate the receipt-blind
// selector view applies in newCommandStateView: output crosses the boundary only
// under a recognized version and applied status, and re-applying the same typed
// paths is idempotent. Omitted, unversioned, and unknown-version entries yield
// no output (srd038 R5.6, srd033 R7.4).
func monitorResolveEntryOutput(res ResultDigest) (string, bool) {
	if res.RedactionVersion == OutputRedactionVersion1 &&
		res.RedactionStatus == OutputRedactionApplied {
		output, _, status := applyOutputRedaction(
			res.Output, res.RedactionVersion, res.RedactedPaths,
		)
		return output, status == OutputRedactionApplied
	}
	return "", false
}
