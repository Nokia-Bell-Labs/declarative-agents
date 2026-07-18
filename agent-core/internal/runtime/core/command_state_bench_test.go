// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	benchBlobSize = 100 * 1024 // ~100KB per step output
	benchSteps    = 32
)

func largeBlobExecution(steps, blobSize int) Execution {
	blob := strings.Repeat("x", blobSize)
	exec := make(Execution, 0, steps)
	for i := 0; i < steps; i++ {
		exec = append(exec, Entry{
			Iteration:   i + 1,
			CommandName: fmt.Sprintf("step-%d", i),
			FromState:   "Working",
			ToState:     "Working",
			Signal:      LLMResponded,
			Result:      ResultDigest{Signal: LLMResponded, Output: fmt.Sprintf(`{"blob":%q}`, blob)},
		})
	}
	return exec
}

// BenchmarkDoltCommandStateLargeBlobPerStep measures the commit-per-step write
// cost of large tool_outputs blobs. It reports the bytes written to tool_outputs
// per step and per run: commit-per-step writes only the current step's row, so
// the write-layer cost is bounded by the new blob (roughly linear in new data),
// not the cumulative history. Actual bytes-on-disk with Dolt prolly-tree chunk
// dedup requires the gated integration environment; this measures the input the
// adapter hands Dolt per commit.
func BenchmarkDoltCommandStateLargeBlobPerStep(b *testing.B) {
	exec := largeBlobExecution(benchSteps, benchBlobSize)
	var perStep, perRun float64
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		db := newFakeDB()
		cp := NewDoltCheckpoint(db, "run-bench", nil)
		for i := 1; i <= benchSteps; i++ {
			if err := cp.Save(samplePosition(), exec[:i]); err != nil {
				b.Fatal(err)
			}
		}
		perStep = float64(db.outputBytes) / float64(benchSteps)
		perRun = float64(db.outputBytes)
	}
	b.ReportMetric(perStep, "outbytes/step")
	b.ReportMetric(perRun, "outbytes/run")
}

// TestDoltCommandStateWritesOnlyNewStepBlob locks the linear write behavior the
// benchmark measures: each Save writes only the current step's tool_outputs row,
// so cumulative write bytes grow ~linearly with the number of steps rather than
// quadratically. A regression to rewriting the full history each commit (which
// would make large-blob runs quadratic) fails here, deciding against a
// content-addressing follow-up at the application layer.
func TestDoltCommandStateWritesOnlyNewStepBlob(t *testing.T) {
	t.Parallel()
	const steps = 8
	exec := largeBlobExecution(steps, benchBlobSize)

	db := newFakeDB()
	cp := NewDoltCheckpoint(db, "run-1", nil)
	for i := 1; i <= steps; i++ {
		require.NoError(t, cp.Save(samplePosition(), exec[:i]))
	}

	// Linear: total is at least one blob per step and well under two blobs per
	// step (quadratic accumulation would be ~steps/2 blobs per step).
	require.GreaterOrEqual(t, db.outputBytes, steps*benchBlobSize)
	require.Less(t, db.outputBytes, steps*(benchBlobSize+2048))
}
