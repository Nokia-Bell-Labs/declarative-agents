// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"path/filepath"
	"testing"
)

// TestLifecycleHistoryRollback drives the standalone history and rollback
// lifecycle families (rel02.0-uc002) against a checkpoint another run persisted.
// A suspend run leaves a resumable run branch; the history and rollback profiles
// then target that branch through the universal --request file (a checkpoint id
// and, for rollback, a to_iteration). Before agent-core routed the request into
// the checkpoint backend and the tool configs, these families had no way to name
// a target run and failed to load one; this proves the request-driven path.
//
// Dolt-gated like the suspend/resume test: it starts a throwaway dolt sql-server
// and skips where dolt is not installed. The checkpoint read/rollback tools run
// against a backend pinned to the target run, so the inspecting machine persists
// to its own throwaway run rather than over the run it inspects.
//
// Traces srd009-lifecycle: R1.3 (history and rollback as visible machine
// actions), R2.2 (the checkpoint tool families), R2.3 (the checkpoint store),
// and R3.2 (Done terminal outcomes).
func TestLifecycleHistoryRollback(t *testing.T) {
	RequireCoreRoot(t)
	dolt := StartDolt(t)

	// A suspend run persists a resumable run branch to target. It leaves a single
	// persisted step (the suspend at iteration 1), the natural shape a lifecycle
	// run leaves behind for later inspection and rollback.
	suspendRes := Run(t, RunConfig{
		Profile: filepath.Join("agents", "lifecycle", "approval", "profile.yaml"),
		Args:    []string{"--dolt-dsn", dolt.DSN()},
	})
	suspendRes.RequireExit(t, 0)
	suspendRes.RequireNoErrorSpans(t)
	runID := dolt.LatestRunBranch(t)
	if runID == "" {
		t.Fatalf("no persisted run branch after suspend\noutput:\n%s", suspendRes.Output)
	}

	t.Run("History", func(t *testing.T) {
		req := writeEphemeral(t, t.TempDir(), "request.yaml", "checkpoint: "+runID+"\n")
		res := Run(t, RunConfig{
			Profile: filepath.Join("agents", "lifecycle", "history", "profile.yaml"),
			Request: req,
			Args:    []string{"--dolt-dsn", dolt.DSN()},
		})

		// srd009 R1.3/R2.2/R3.2: checkpoint_history reads the target run branch
		// named by the request and the machine reaches a clean Done terminal.
		res.RequireExit(t, 0)
		res.RootRequired(t)
		res.RequireNoErrorSpans(t)
		res.RequireToolSpans(t, "checkpoint_history")
		res.RequireTerminalState(t, "Done")
	})

	t.Run("Rollback", func(t *testing.T) {
		req := writeEphemeral(t, t.TempDir(), "request.yaml", "checkpoint: "+runID+"\nto_iteration: 1\n")
		res := Run(t, RunConfig{
			Profile: filepath.Join("agents", "lifecycle", "rollback", "profile.yaml"),
			Request: req,
			Args:    []string{"--dolt-dsn", dolt.DSN()},
		})

		// srd009 R1.3/R2.2/R3.2: checkpoint_rollback reverts the target run to the
		// requested iteration and the machine reaches a clean Done terminal.
		res.RequireExit(t, 0)
		res.RootRequired(t)
		res.RequireNoErrorSpans(t)
		res.RequireToolSpans(t, "checkpoint_rollback")
		res.RequireTerminalState(t, "Done")
	})
}
