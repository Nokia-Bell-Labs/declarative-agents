// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWriteGeneratorChildAgentExercisesGeneratorProfile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell shim is POSIX-only")
	}
	binDir := t.TempDir()
	workspace := t.TempDir()
	trace := filepath.Join(t.TempDir(), "trace.ndjson")
	writeFile(t, filepath.Join(workspace, "greet.go"), "package greet\n\nfunc Hello(name string) string { return \"\" }\n")

	if err := writeGeneratorChildAgent(binDir); err != nil {
		t.Fatalf("writeGeneratorChildAgent: %v", err)
	}
	cmd := exec.Command(filepath.Join(binDir, "agent"),
		"--profile", "/profiles/agents/executor/profile.yaml",
		"--directory", workspace,
		"--otel-log-file", trace,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child agent shim failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "generator profile boundary exercised") {
		t.Fatalf("child output missing boundary marker: %s", out)
	}
	if got := readTestFile(t, filepath.Join(workspace, "greet.go")); !strings.Contains(got, `return "Hello, " + name + "!"`) {
		t.Fatalf("greet.go was not updated:\n%s", got)
	}
	if got := readTestFile(t, trace); !strings.Contains(got, "gen_ai.usage.input_tokens") {
		t.Fatalf("trace missing token evidence:\n%s", got)
	}
}

func TestAssertEvaluatorGeneratorOutputRequiresBoundaryEvidence(t *testing.T) {
	outputDir := t.TempDir()
	pointDir := filepath.Join(outputDir, "rel07", "greet--generator--unknown--rep1")
	writeFile(t, filepath.Join(pointDir, "meta.json"), `{
  "harness": "generator",
  "model": "unknown",
  "sample": "greet",
  "exit_code": 0,
  "tests_passed": true,
  "timed_out": false,
  "test_output": "ok"
}`)
	writeFile(t, filepath.Join(pointDir, "experiment.yaml"), "profile: /profiles/agents/executor/profile.yaml\n")
	writeFile(t, filepath.Join(pointDir, "greet.go"), `package greet

func Hello(name string) string {
	return "Hello, " + name + "!"
}
`)
	writeFile(t, filepath.Join(pointDir, "trace.ndjson"), `{"Name":"child generator shim","Attributes":[{"Key":"gen_ai.usage.input_tokens","Value":{"Type":"INT64","Value":1}}]}`+"\n")

	if err := assertEvaluatorGeneratorOutput(outputDir); err != nil {
		t.Fatalf("assertEvaluatorGeneratorOutput returned error: %v", err)
	}
}

func TestAssertEvaluatorGeneratorOutputRejectsMissingOraclePass(t *testing.T) {
	outputDir := t.TempDir()
	pointDir := filepath.Join(outputDir, "rel07", "greet--generator--unknown--rep1")
	writeFile(t, filepath.Join(pointDir, "meta.json"), `{
  "harness": "generator",
  "model": "unknown",
  "sample": "greet",
  "exit_code": 0,
  "tests_passed": false,
  "timed_out": false,
  "test_output": "FAIL"
}`)
	writeFile(t, filepath.Join(pointDir, "experiment.yaml"), "profile: /profiles/agents/executor/profile.yaml\n")
	writeFile(t, filepath.Join(pointDir, "greet.go"), `package greet

func Hello(name string) string {
	return "Hello, " + name + "!"
}
`)
	writeFile(t, filepath.Join(pointDir, "trace.ndjson"), `{"Name":"child generator shim","Attributes":[{"Key":"gen_ai.usage.input_tokens","Value":{"Type":"INT64","Value":1}}]}`+"\n")

	err := assertEvaluatorGeneratorOutput(outputDir)
	if err == nil {
		t.Fatal("expected oracle failure")
	}
	if !strings.Contains(err.Error(), "oracle did not pass") {
		t.Fatalf("error = %q, want oracle failure", err)
	}
}

func TestEvaluatorPointDirsFindsMetaFiles(t *testing.T) {
	outputDir := t.TempDir()
	writeFile(t, filepath.Join(outputDir, "session", "point-a", "meta.json"), "{}")
	writeFile(t, filepath.Join(outputDir, "session", "point-b", "meta.json"), "{}")

	points, err := evaluatorPointDirs(outputDir)
	if err != nil {
		t.Fatalf("evaluatorPointDirs: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("points = %v, want two point dirs", points)
	}
	for _, point := range points {
		if _, err := os.Stat(filepath.Join(point, "meta.json")); err != nil {
			t.Fatalf("point missing meta.json: %s", point)
		}
	}
}
