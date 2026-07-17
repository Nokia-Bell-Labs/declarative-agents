// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"path/filepath"
	"testing"
)

// TestLifecycleApprovalSuspendResume drives the approval lifecycle profile
// across the two CLI invocations the suspend/resume flow requires: run one
// suspends at the approval gate and persists a checkpoint through the Dolt
// backend, and run two resumes that checkpoint with an explicit signal. It
// covers both the Approved path (reaching Succeeded) and the Rejected path
// (reaching Rejected), the headline behavior of rel02.0-uc001.
//
// A live checkpoint backend is required: suspend persists only when --dolt-dsn
// names a running dolt sql-server, and resume reloads the persisted Position by
// branch id. The test starts a throwaway server (StartDolt) and skips where
// dolt is not installed, mirroring how agent-core's own Dolt round-trip test is
// gated on a server. History and rollback (rel02.0-uc002) drive the standalone
// history/rollback profiles through a --request checkpoint id rather than the
// resume path and are tracked separately.
//
// Traces srd009-lifecycle: R1.3 (suspend, resume approval, resume rejection,
// and terminal outcomes as visible machine signals), R2.2 (suspend and done
// tool families), R2.3 (agent-core lifecycle ports and the checkpoint store),
// and R3.2 (Succeeded and Rejected terminal outcomes).
func TestLifecycleApprovalSuspendResume(t *testing.T) {
	RequireCoreRoot(t)
	dolt := StartDolt(t)
	profile := filepath.Join("agents", "lifecycle", "approval", "profile.yaml")

	// suspend runs the profile with no resume signal so it dispatches the
	// suspend tool, persists the checkpoint, and exits suspended. It asserts the
	// clean suspend boundary and returns the persisted run branch id for resume.
	suspend := func(t *testing.T) string {
		t.Helper()
		res := Run(t, RunConfig{Profile: profile, Args: []string{"--dolt-dsn", dolt.DSN()}})

		// srd009 R1.3/R2.2: a clean suspend boundary emits the suspend tool span
		// and the run.suspended signal with no error-status spans.
		res.RequireExit(t, 0)
		res.RootRequired(t)
		res.RequireNoErrorSpans(t)
		res.RequireToolSpans(t, "suspend")
		if _, _, ok := res.Spans.FindEvent("run.suspended"); !ok {
			t.Fatalf("no run.suspended event; span names: %v\noutput:\n%s", res.Spans.Names(), res.Output)
		}

		// srd009 R2.3: the checkpoint store persisted a resumable run branch.
		runID := dolt.LatestRunBranch(t)
		if runID == "" {
			t.Fatalf("no persisted run branch after suspend\noutput:\n%s", res.Output)
		}
		return runID
	}

	t.Run("Approved", func(t *testing.T) {
		runID := suspend(t)
		res := Run(t, RunConfig{Profile: profile, Args: []string{
			"--dolt-dsn", dolt.DSN(),
			"--resume-checkpoint", runID,
			"--resume-signal", "Approved",
		}})

		// srd009 R1.3/R3.2: the approved signal re-enters the machine and drives
		// it to the Succeeded terminal via the done tool, with no error spans.
		res.RequireExit(t, 0)
		res.RootRequired(t)
		res.RequireNoErrorSpans(t)
		res.RequireToolSpans(t, "done")
		res.RequireTerminalState(t, "Succeeded")
	})

	t.Run("Rejected", func(t *testing.T) {
		runID := suspend(t)
		res := Run(t, RunConfig{Profile: profile, Args: []string{
			"--dolt-dsn", dolt.DSN(),
			"--resume-checkpoint", runID,
			"--resume-signal", "Rejected",
		}})

		// srd009 R1.3/R3.2: the rejected signal re-enters the machine and reaches
		// the Rejected terminal directly, a clean non-error outcome.
		res.RequireExit(t, 0)
		res.RootRequired(t)
		res.RequireNoErrorSpans(t)
		res.RequireTerminalState(t, "Rejected")
	})
}
