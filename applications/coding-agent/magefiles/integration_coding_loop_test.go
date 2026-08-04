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
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, demoConfigFile), []byte("ollama_url: http://127.0.0.1:1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reason := liveSkipReason(integrationRoots{Application: root})
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
	appRoot := filepath.Join(t.TempDir(), "test")
	profilesRoot := t.TempDir()
	writeTestFile(t, filepath.Join(appRoot, "agents", "application.yaml"), `schema_version: 1
application: test
ownership: agent-owning
module_status: implemented
capabilities:
  runnable_module: {status: implemented, evidence: [test]}
  packaged: {status: implemented, evidence: [test]}
roots:
  - {id: executor, ownership: catalog, source: agents/executor/profile.yaml, runtime_path: agents/executor/profile.yaml, compatible_release: v0.test}
  - {id: planner, ownership: catalog, source: agents/planner/profile.yaml, runtime_path: agents/planner/profile.yaml, compatible_release: v0.test}
  - {id: critic, ownership: catalog, source: agents/critic/profile.yaml, runtime_path: agents/critic/profile.yaml, compatible_release: v0.test}
  - {id: critic-workspace, ownership: catalog, source: agents/critic/profile-workspace.yaml, runtime_path: agents/critic/profile-workspace.yaml, compatible_release: v0.test}
  - {id: coding-planner-server, ownership: local, source: agents/planner/profile.yaml, runtime_path: applications/coding-agent/planner/profile.yaml}
  - {id: coding-executor-server, ownership: local, source: agents/executor/profile.yaml, runtime_path: applications/coding-agent/executor/profile.yaml}
  - {id: coding-critic-server, ownership: local, source: agents/critic/profile.yaml, runtime_path: applications/coding-agent/critic/profile.yaml}
  - {id: applier, ownership: local, source: agents/applier/profile.yaml, runtime_path: applications/coding-agent/applier/profile.yaml}
runtime:
  mount_path: /profiles
  image_contains_profiles: false
deployment:
  entries:
    - {id: planner, root: coding-planner-server}
    - {id: executor, root: coding-executor-server}
    - {id: critic, root: coding-critic-server}
    - {id: applier, root: applier}
`)
	for _, actor := range []string{"planner", "executor", "critic", "applier"} {
		writeTestFile(t, filepath.Join(appRoot, "agents", actor, "profile.yaml"), "name: "+actor+"\n")
	}
	sourceProfile := filepath.Join(profilesRoot, "agents", "executor", "profile.yaml")
	writeTestFile(t, sourceProfile, "name: packaged-executor\n")
	writeTestFile(t, filepath.Join(profilesRoot, "agents", "planner", "profile.yaml"), "name: packaged-planner\n")
	writeTestFile(t, filepath.Join(profilesRoot, "agents", "critic", "profile.yaml"), "name: packaged-critic\n")
	writeTestFile(t, filepath.Join(profilesRoot, "agents", "critic", "profile-workspace.yaml"), "name: packaged-critic-workspace\n")
	coreRoot := t.TempDir()
	writeTestFile(t, filepath.Join(coreRoot, "go.mod"), "module test-core\n\ngo 1.26\n")

	packaged, cleanup, err := packageIntegrationRoots(integrationRoots{
		Application: appRoot,
		Core:        coreRoot,
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
	if reason := baseIntegrationSkipReason(packaged); reason != "" {
		t.Fatalf("packaged profile root was misclassified as unavailable: %s", reason)
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

func TestObservableGreetingValidationAcceptsEquivalentImplementation(t *testing.T) {
	workspace := t.TempDir()
	writeTestFile(t, filepath.Join(workspace, "go.mod"), "module greet\n\ngo 1.26\n")
	writeTestFile(t, filepath.Join(workspace, "greet.go"), "package greet\n\nimport \"fmt\"\n\nfunc Hello(name string) string { return fmt.Sprintf(\"Hello, %s!\", name) }\n")
	writeTestFile(t, filepath.Join(workspace, "greet_test.go"), "package greet\n\nimport \"testing\"\n\nfunc TestHello(t *testing.T) { if Hello(\"Go\") != \"Hello, Go!\" { t.Fail() } }\n")
	if err := requireGreetingAndTests(workspace); err != nil {
		t.Fatalf("requireGreetingAndTests rejected equivalent implementation: %v", err)
	}
	if err := requireSuccessfulExecutor(workspace, agentRun{Output: "terminal state: Succeeded\n"}); err != nil {
		t.Fatalf("requireSuccessfulExecutor rejected equivalent implementation: %v", err)
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
