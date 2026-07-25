// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOllamaSkipReasonRequiresCanonicalModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"models":[{"name":"other:model"}]}`))
	}))
	defer server.Close()
	reason := ollamaSkipReason(server.Client(), server.URL, canonicalModel)
	if !strings.Contains(reason, canonicalModel) {
		t.Fatalf("skip reason = %q, want missing canonical model", reason)
	}
}

func TestOllamaSkipReasonAcceptsCanonicalModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"models":[{"model":"qwen3.6:35b-mlx"}]}`))
	}))
	defer server.Close()
	if reason := ollamaSkipReason(server.Client(), server.URL, canonicalModel); reason != "" {
		t.Fatalf("skip reason = %q, want runnable", reason)
	}
}

func TestLiveSkipReasonRejectsNonCanonicalProbeEndpoint(t *testing.T) {
	t.Setenv(ollamaProbeURLEnv, "http://127.0.0.1:1")
	reason := liveSkipReason(integrationRoots{})
	if !strings.Contains(reason, "does not match canonical profile endpoint") {
		t.Fatalf("skip reason = %q", reason)
	}
}

func TestTraceFinalStateReadsMachineTerminal(t *testing.T) {
	trace := `{"Attributes":[{"Key":"run.final_state","Value":{"Type":"STRING","Value":"Completed"}}]}`
	if got := traceFinalState(trace); got != "Completed" {
		t.Fatalf("traceFinalState = %q, want Completed", got)
	}
}

func TestFreshWorkspaceIsPortableAndIsolated(t *testing.T) {
	appRoot := filepath.Clean(filepath.Join(".."))
	workspace, cleanup, err := freshWorkspace(appRoot)
	if err != nil {
		t.Fatalf("freshWorkspace: %v", err)
	}
	defer cleanup()
	for _, rel := range []string{
		"go.mod",
		"greet.go",
		"greet_test.go",
		filepath.Join("doc", "specs", "software-requirements", "srd001-greet.yaml"),
		filepath.Join("docs", "SPECIFICATIONS.yaml"),
	} {
		if _, err := os.Stat(filepath.Join(workspace, rel)); err != nil {
			t.Errorf("fresh workspace missing %s: %v", rel, err)
		}
	}
	if err := os.WriteFile(filepath.Join(workspace, "greet.go"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(appRoot, "testdata", "integration", "coding-loop", "workspace", "greet.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(source) == "changed" {
		t.Fatal("fresh workspace mutation changed the fixture")
	}
}

func TestRequireSuccessfulExecutorChecksTerminalEditAndTests(t *testing.T) {
	workspace := t.TempDir()
	writeTestFile(t, filepath.Join(workspace, "go.mod"), "module greet\n\ngo 1.26\n")
	writeTestFile(t, filepath.Join(workspace, "greet.go"), "package greet\n\nfunc Hello(name string) string { return \"Hello, \" + name + \"!\" }\n")
	writeTestFile(t, filepath.Join(workspace, "greet_test.go"), "package greet\n\nimport \"testing\"\n\nfunc TestHello(t *testing.T) { if Hello(\"Go\") != \"Hello, Go!\" { t.Fail() } }\n")
	if err := requireSuccessfulExecutor(workspace, agentRun{Output: "terminal state: Succeeded\n"}); err != nil {
		t.Fatalf("requireSuccessfulExecutor: %v", err)
	}
}

func TestRequirePassingCriticEvidenceRejectsFailedOracle(t *testing.T) {
	output := t.TempDir()
	point := filepath.Join(output, "session", "point")
	writeTestFile(t, filepath.Join(point, "meta.json"), `{
  "harness": "executor",
  "tests_passed": false,
  "timed_out": false,
  "exit_code": 0,
  "tokens": 10
}`)
	if err := requirePassingCriticEvidence(output); err == nil {
		t.Fatal("expected failed oracle evidence to be rejected")
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
