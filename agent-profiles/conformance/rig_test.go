// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// rigBinDir stages the built agent binary as "agent" in a temp dir, so the
// assembler's children — twins, subjects, validators — resolve it from PATH.
func rigBinDir(t *testing.T) string {
	t.Helper()
	coreRoot := RequireCoreRoot(t)
	built := agentBinary(t, coreRoot)
	dir := t.TempDir()
	dst := filepath.Join(dir, "agent")
	src, err := os.Open(built)
	if err != nil {
		t.Fatalf("open built agent: %v", err)
	}
	defer func() { _ = src.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		t.Fatalf("stage agent binary: %v", err)
	}
	if _, err := io.Copy(out, src); err != nil {
		t.Fatalf("copy agent binary: %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("close staged binary: %v", err)
	}
	return dir
}

// rigVerdictSignals returns the collect_scenario_verdict signals in execution
// order, which is the discovery order: scenarios sorted by subject then name.
func rigVerdictSignals(result RunResult) []string {
	var signals []string
	for _, span := range result.Spans {
		if span.Name != "execute_tool collect_scenario_verdict" {
			continue
		}
		for _, attr := range span.Attributes {
			if attr.Key == "command.signal" {
				if text, ok := attr.Value.Value.(string); ok {
					signals = append(signals, text)
				}
			}
		}
	}
	return signals
}

// TestRigSelfProof runs the assembler over the shipped tree and asserts the
// reference subject's three scenarios land exactly as designed: happy-path
// and dep-failure pass, and the deliberately broken expectation fails — so the
// rig is proven able to fail, not only to pass. The aggregate is therefore
// failed, which is this test's expected outcome, and the run is repeated to
// prove ports and state do not leak between runs.
//
// Traces rel11.0-uc001-assembler-scenario-run S1-S5 and srd018 AC1, AC2, AC6.
func TestRigSelfProof(t *testing.T) {
	binDir := rigBinDir(t)
	pathEnv := "PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH")

	for run := 1; run <= 2; run++ {
		t.Run(map[int]string{1: "first", 2: "second"}[run], func(t *testing.T) {
			result := Run(t, RunConfig{
				Profile: filepath.Join("agents", "assembler", "profile.yaml"),
				Env:     []string{pathEnv},
				Timeout: 3 * time.Minute,
			})

			// The binary exits zero either way (#683); the terminal status line
			// carries the aggregate. Failed is expected: broken must fail.
			result.RequireExit(t, 0)
			if !strings.Contains(result.Output, "terminal state: failed") {
				t.Fatalf("aggregate should be failed (the broken scenario must fail):\n%s", result.Output)
			}

			// Discovery order is sorted: broken, dep-failure, happy-path.
			verdicts := rigVerdictSignals(result)
			want := []string{"ScenarioFailed", "ScenarioPassed", "ScenarioPassed"}
			if len(verdicts) != len(want) {
				t.Fatalf("verdicts = %v, want %v\noutput:\n%s", verdicts, want, result.Output)
			}
			for i := range want {
				if verdicts[i] != want[i] {
					t.Fatalf("verdict[%d] = %s, want %s (order: broken, dep-failure, happy-path)\nall: %v",
						i, verdicts[i], want[i], verdicts)
				}
			}
		})
	}
}
