// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// BenchmarkDoltCheckpointSQLPayloadPerStep measures bytes presented to the SQL
// write seam. It is an application payload benchmark, not a storage benchmark.
func BenchmarkDoltCheckpointSQLPayloadPerStep(b *testing.B) {
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

// BenchmarkDoltRepositoryGrowthLargeBlob measures real Dolt repository growth
// for repeated and distinct deterministic blobs. Enable it explicitly because
// it requires the dolt executable and performs filesystem-heavy commits:
//
//	AGENT_CORE_DOLT_BENCH=1 go test ./internal/runtime/core -run '^$' \
//	  -bench BenchmarkDoltRepositoryGrowthLargeBlob -benchtime=1x
func BenchmarkDoltRepositoryGrowthLargeBlob(b *testing.B) {
	if os.Getenv("AGENT_CORE_DOLT_BENCH") != "1" {
		b.Skip("set AGENT_CORE_DOLT_BENCH=1 to run the real Dolt storage benchmark")
	}
	dolt, err := exec.LookPath("dolt")
	if err != nil {
		b.Skipf("dolt is not installed: %v", err)
	}
	for _, tc := range []struct {
		name string
		blob func(int) string
	}{
		{name: "duplicate-content", blob: func(int) string { return strings.Repeat("x", benchBlobSize) }},
		{name: "distinct-content", blob: func(step int) string {
			return strings.Repeat(string(rune('a'+step%26)), benchBlobSize)
		}},
	} {
		b.Run(tc.name, func(b *testing.B) {
			var growth int64
			for n := 0; n < b.N; n++ {
				b.StopTimer()
				root, env := initDoltBenchmarkRepo(b, dolt)
				before := directoryBytes(b, filepath.Join(root, ".dolt"))
				b.StartTimer()
				for step := 0; step < benchSteps; step++ {
					runDoltBenchmark(b, dolt, root, env, "sql", "-q", fmt.Sprintf(
						"INSERT INTO tool_outputs VALUES (%d, '%s')", step, tc.blob(step),
					))
					runDoltBenchmark(b, dolt, root, env, "add", ".")
					runDoltBenchmark(b, dolt, root, env, "commit", "-m", fmt.Sprintf("step %d", step))
				}
				b.StopTimer()
				growth += directoryBytes(b, filepath.Join(root, ".dolt")) - before
				require.NoError(b, os.RemoveAll(root))
			}
			b.ReportMetric(float64(growth)/float64(b.N), "repo-growth-bytes/run")
			b.ReportMetric(float64(growth)/float64(b.N*benchSteps), "repo-growth-bytes/step")
		})
	}
}

func initDoltBenchmarkRepo(b *testing.B, dolt string) (string, []string) {
	b.Helper()
	root, err := os.MkdirTemp("", "dolt-storage-bench-*")
	require.NoError(b, err)
	home := filepath.Join(root, "home")
	require.NoError(b, os.MkdirAll(home, 0o755))
	env := append(os.Environ(), "DOLT_ROOT_PATH="+home)
	runDoltBenchmark(b, dolt, root, env, "config", "--global", "--add", "user.name", "benchmark")
	runDoltBenchmark(b, dolt, root, env, "config", "--global", "--add", "user.email", "benchmark@example.com")
	runDoltBenchmark(b, dolt, root, env, "init")
	runDoltBenchmark(b, dolt, root, env, "sql", "-q", "CREATE TABLE tool_outputs (step_index INT PRIMARY KEY, output LONGTEXT)")
	runDoltBenchmark(b, dolt, root, env, "add", ".")
	runDoltBenchmark(b, dolt, root, env, "commit", "-m", "schema")
	return root, env
}

func runDoltBenchmark(b *testing.B, dolt, dir string, env []string, args ...string) {
	b.Helper()
	cmd := exec.Command(dolt, args...)
	cmd.Dir = dir
	cmd.Env = env
	if output, err := cmd.CombinedOutput(); err != nil {
		b.Fatalf("dolt %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func directoryBytes(b *testing.B, root string) int64 {
	b.Helper()
	var size int64
	err := filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			size += info.Size()
		}
		return nil
	})
	require.NoError(b, err)
	return size
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
