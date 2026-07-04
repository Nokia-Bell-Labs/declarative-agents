// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadBenchLaunchRequestParsesAction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "request.yaml")
	writeFile(t, path, `type: launch_eval
config:
  suite: suite.yaml
  output_dir: eval-results
  request_id: rel07-bench-evaluator
`)

	request, err := readBenchLaunchRequest(path)
	if err != nil {
		t.Fatalf("readBenchLaunchRequest: %v", err)
	}
	if request.Type != "launch_eval" {
		t.Fatalf("type = %q", request.Type)
	}
	if got := fixtureValue(request.Config, "suite"); got != "suite.yaml" {
		t.Fatalf("suite = %q", got)
	}
}

func TestWriteEvaluatorChildAgentRecordsEvaluatorOutput(t *testing.T) {
	runDir := t.TempDir()
	evaluator, err := writeEvaluatorChildAgent(runDir)
	if err != nil {
		t.Fatalf("writeEvaluatorChildAgent: %v", err)
	}
	outputDir := filepath.Join(runDir, "eval-results")
	cmd := exec.Command(evaluator,
		"--profile", "/profiles/agents/evaluator/profile.yaml",
		"--request", "/fixtures/suite.yaml",
		"--output", outputDir,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("evaluator shim failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "evaluator profile boundary exercised") {
		t.Fatalf("shim output missing marker: %s", out)
	}
	summary := readTestFile(t, filepath.Join(outputDir, "session-summary.yaml"))
	for _, want := range []string{"/profiles/agents/evaluator/profile.yaml", "/fixtures/suite.yaml", "status: completed"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}
}

func TestAssertBenchEvaluatorEvidenceRequiresLaunchOutput(t *testing.T) {
	runDir := t.TempDir()
	evidence := benchLaunchEvidence{
		RequestID:                   "rel07-bench-evaluator",
		HumanActionReceived:         true,
		EvaluatorProfile:            "agents/evaluator/profile.yaml",
		Suite:                       "suite.yaml",
		LaunchStatus:                "completed",
		EvaluatorOutputEvidence:     "eval-results/session-summary.yaml",
		EvaluatorOutputMaterialized: true,
	}
	if err := writeBenchLaunchEvidence(runDir, evidence); err != nil {
		t.Fatalf("writeBenchLaunchEvidence: %v", err)
	}
	writeFile(t, filepath.Join(runDir, "eval-results", "session-summary.yaml"), `profile: /profiles/agents/evaluator/profile.yaml
suite: /fixtures/suite.yaml
status: completed
points: 1
`)

	if err := assertBenchEvaluatorEvidence(runDir, evidence); err != nil {
		t.Fatalf("assertBenchEvaluatorEvidence: %v", err)
	}
}

func TestAssertBenchEvaluatorEvidenceRejectsMissingSummary(t *testing.T) {
	runDir := t.TempDir()
	evidence := benchLaunchEvidence{
		RequestID:                   "rel07-bench-evaluator",
		HumanActionReceived:         true,
		EvaluatorProfile:            "agents/evaluator/profile.yaml",
		Suite:                       "suite.yaml",
		LaunchStatus:                "completed",
		EvaluatorOutputEvidence:     "eval-results/session-summary.yaml",
		EvaluatorOutputMaterialized: true,
	}
	if err := writeBenchLaunchEvidence(runDir, evidence); err != nil {
		t.Fatalf("writeBenchLaunchEvidence: %v", err)
	}

	err := assertBenchEvaluatorEvidence(runDir, evidence)
	if err == nil {
		t.Fatal("expected missing summary error")
	}
	if !strings.Contains(err.Error(), "read evaluator output evidence") {
		t.Fatalf("error = %q", err)
	}
}
