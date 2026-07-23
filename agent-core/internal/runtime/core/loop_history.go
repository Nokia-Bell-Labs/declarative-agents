// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"encoding/json"
	"strings"
	"time"
)

// resolveCheckpoint returns the configured Checkpoint port, or NoopCheckpoint
// when none is injected so disabled-mode execution keeps its current behavior
// (srd035-checkpoint-port R5.1).
func resolveCheckpoint(c Checkpoint) Checkpoint {
	if c == nil {
		return NoopCheckpoint{}
	}
	return c
}

// dispatchPosition builds the resumable Position from loop-owned state. The
// conversation is folded into the snapshot by the domain that owns it (core
// cannot import the llm package); loop-owned code leaves it empty.
func dispatchPosition(state State, signal Signal, iteration int, rr *RunResult) Position {
	return Position{
		CurrentState: state,
		LastSignal:   signal,
		Snapshot: AgentSnapshot{
			State:     state,
			Signal:    signal,
			Iteration: iteration,
			TokensIn:  rr.TokensIn,
			TokensOut: rr.TokensOut,
			TotalCost: rr.TotalCost,
		},
	}
}

// dispatchEntry builds the Execution entry for one completed dispatch. Signal is
// the transition signal that selected the command; label is the optional stable
// address authored on that transition. Receipt is the tool-owned opaque receipt
// carried verbatim from the Result (srd035 R2.4, R3; srd038 R1.6, R2).
func dispatchEntry(iteration int, fromState, toState State, transitionSignal Signal, label string, res Result) Entry {
	return Entry{
		Iteration:   iteration,
		Timestamp:   time.Now().UTC(),
		CommandName: res.CommandName,
		Label:       label,
		FromState:   fromState,
		ToState:     toState,
		Signal:      transitionSignal,
		Result:      DigestResult(res),
		Receipt:     res.Receipt,
	}
}

// DigestResult applies the core output-redaction boundary and returns the
// serializable forward-plane projection of a Result. Synthetic execution
// producers use this same boundary as loop dispatch (srd035 R7, srd038 R5).
func DigestResult(res Result) ResultDigest {
	version := res.Redaction.Version
	if version == 0 && len(res.Redaction.Paths) == 0 {
		version = OutputRedactionVersion1
	}
	output, paths, status := applyOutputRedaction(res.Output, version, res.Redaction.Paths)
	digest := ResultDigest{
		Signal:           res.Signal,
		Output:           output,
		Cost:             res.Cost,
		RedactionVersion: version,
		RedactedPaths:    paths,
		RedactionStatus:  status,
	}
	if res.Err != nil {
		digest.Error = res.Err.Error()
	}
	return digest
}

// applyOutputRedaction removes typed paths from JSON object output. It returns
// whole-output omission for unsafe metadata or shape mismatches. Missing paths
// are successful so applying the same metadata again is idempotent.
func applyOutputRedaction(
	output string,
	version uint16,
	paths []OutputRedactionPath,
) (string, []OutputRedactionPath, OutputRedactionStatus) {
	if version != OutputRedactionVersion1 || !validOutputRedactionPaths(paths) {
		return "", nil, OutputRedactionOmitted
	}
	clonedPaths := cloneOutputRedactionPaths(paths)
	if len(paths) == 0 {
		return output, clonedPaths, OutputRedactionApplied
	}

	var object map[string]interface{}
	if err := json.Unmarshal([]byte(output), &object); err != nil || object == nil {
		return "", nil, OutputRedactionOmitted
	}
	for _, path := range paths {
		if !removeOutputPath(object, path) {
			return "", nil, OutputRedactionOmitted
		}
	}
	sanitized, err := json.Marshal(object)
	if err != nil {
		return "", nil, OutputRedactionOmitted
	}
	return string(sanitized), clonedPaths, OutputRedactionApplied
}

func validOutputRedactionPaths(paths []OutputRedactionPath) bool {
	for _, path := range paths {
		if len(path) == 0 {
			return false
		}
		selector := "$." + strings.Join(path, ".")
		parsed, ok := ParseSelector(selector)
		if !ok || len(parsed.Path) != len(path) {
			return false
		}
		for i := range path {
			if parsed.Path[i] != path[i] {
				return false
			}
		}
	}
	return true
}

func removeOutputPath(object map[string]interface{}, path OutputRedactionPath) bool {
	current := object
	for _, segment := range path[:len(path)-1] {
		next, exists := current[segment]
		if !exists {
			return true
		}
		nested, ok := next.(map[string]interface{})
		if !ok {
			return false
		}
		current = nested
	}
	delete(current, path[len(path)-1])
	return true
}

func cloneOutputRedactionPaths(paths []OutputRedactionPath) []OutputRedactionPath {
	if len(paths) == 0 {
		return nil
	}
	cloned := make([]OutputRedactionPath, len(paths))
	for i, path := range paths {
		cloned[i] = append(OutputRedactionPath(nil), path...)
	}
	return cloned
}
