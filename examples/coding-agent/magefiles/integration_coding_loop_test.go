// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
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

func TestPackagedIntegrationRootsDoNotObserveCheckoutMutations(t *testing.T) {
	appRoot := t.TempDir()
	profilesRoot := t.TempDir()
	writeTestFile(t, filepath.Join(appRoot, "agents", "application.yaml"), `schema_version: 1
application: test
agent_profiles:
  compatible_release: agent-profiles/v0.test
  references:
    - {role: executor, source: agents/executor/profile.yaml, runtime_path: agents/executor/profile.yaml}
runtime:
  mount_path: /profiles
  image_contains_profiles: false
deployment:
  serving_profiles:
    - {role: planner, source: agents/serving/planner/profile.yaml, runtime_path: applications/coding-agent/planner/profile.yaml}
    - {role: executor, source: agents/serving/executor/profile.yaml, runtime_path: applications/coding-agent/executor/profile.yaml}
    - {role: critic, source: agents/serving/critic/profile.yaml, runtime_path: applications/coding-agent/critic/profile.yaml}
`)
	sourceProfile := filepath.Join(profilesRoot, "agents", "executor", "profile.yaml")
	writeTestFile(t, sourceProfile, "name: packaged-executor\n")

	packaged, cleanup, err := packageIntegrationRoots(integrationRoots{
		Application: appRoot,
		Profiles:    profilesRoot,
	})
	if err != nil {
		t.Fatalf("packageIntegrationRoots: %v", err)
	}
	packageParent := filepath.Dir(packaged.Profiles)
	defer cleanup()
	if packaged.Profiles == profilesRoot {
		t.Fatal("integration profile root still points at the checkout")
	}
	packagedProfile := filepath.Join(packaged.Profiles, "agents", "executor", "profile.yaml")
	before, err := os.ReadFile(packagedProfile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourceProfile, []byte("name: mutated-checkout\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(packagedProfile)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) || strings.Contains(string(after), "mutated-checkout") {
		t.Fatalf("packaged profile observed checkout mutation:\n%s", after)
	}
	cleanup()
	if _, err := os.Stat(packageParent); !os.IsNotExist(err) {
		t.Fatalf("temporary closure still exists after cleanup: %v", err)
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

func TestApplicationOutcomeMapsOnlyCanonicalVerdicts(t *testing.T) {
	accepted := canonicalCriticVerdict{Verdict: "accepted", Accepted: true}
	if got, err := applicationOutcome(accepted); err != nil || got != "Succeeded" {
		t.Fatalf("accepted outcome = %q, %v", got, err)
	}
	rejected := canonicalCriticVerdict{Verdict: "rejected", Accepted: false}
	if got, err := applicationOutcome(rejected); err != nil || got != "Failed" {
		t.Fatalf("rejected outcome = %q, %v", got, err)
	}
	if _, err := applicationOutcome(canonicalCriticVerdict{Verdict: "accepted"}); err == nil {
		t.Fatal("accepted verdict with accepted=false must not map to success")
	}
}

func TestCriticCandidateFixturesHaveOppositeOracleResults(t *testing.T) {
	appRoot := filepath.Clean(filepath.Join(".."))
	for _, tc := range []struct {
		name     string
		wantPass bool
	}{
		{name: "accepted", wantPass: true},
		{name: "rejected", wantPass: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("go", "test", "./...")
			cmd.Dir = filepath.Join(appRoot, "testdata", "integration", "coding-loop", "candidates", tc.name)
			err := cmd.Run()
			if (err == nil) != tc.wantPass {
				t.Fatalf("go test error = %v, wantPass=%t", err, tc.wantPass)
			}
		})
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
