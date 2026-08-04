// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStageServingProfileTreeCombinesClosureAndApplicationProfiles(t *testing.T) {
	app := filepath.Clean("..")
	roots := integrationRoots{
		Application: app,
		Core:        filepath.Clean(filepath.Join(app, "..", "..", "agent-core")),
		Profiles:    filepath.Clean(filepath.Join(app, "..", "catalog")),
	}
	root, cleanup, err := stageServingProfileTree(roots)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	for _, rel := range []string{
		"agents/planner/profile.yaml",
		"agents/executor/profile.yaml",
		"agents/critic/profile-workspace.yaml",
		"applications/coding-agent/role-server/machine.yaml",
		"applications/coding-agent/planner/profile.yaml",
		"applications/coding-agent/executor/profile.yaml",
		"applications/coding-agent/critic/profile.yaml",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("staged serving tree missing %s: %v", rel, err)
		}
	}
}

func TestServingProfilesUseRealLifecycleAndCanonicalRoleProfiles(t *testing.T) {
	root := filepath.Join("..", "agents")
	for _, role := range servingRoles {
		data, err := os.ReadFile(filepath.Join(root, role, "rest.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		// The lifecycle exit route is injected per served agent by agent-core
		// (GH-1264), so serving profiles no longer declare /api/lifecycle/exit
		// themselves; they still declare the health probe and the request binding.
		for _, want := range []string{
			"binding: machine_request",
			"path: /api/lifecycle/health",
			"binding: health",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("%s serving profile missing %q", role, want)
			}
		}
		if strings.Contains(text, "path: /api/lifecycle/exit") {
			t.Errorf("%s serving profile declares /api/lifecycle/exit; it should rely on the injected endpoint", role)
		}
	}
	executor := readServingFile(t, root, "executor", "rest.yaml")
	if !strings.Contains(executor, "profile: agents/executor/profile.yaml") {
		t.Error("executor server does not run the canonical executor profile")
	}
	critic := readServingFile(t, root, "critic", "rest.yaml")
	if !strings.Contains(critic, "profile: agents/critic/profile-workspace.yaml") {
		t.Error("critic server does not run the canonical changed-workspace profile")
	}
	planner := readServingFile(t, root, "planner", "request-profile.yaml")
	for _, want := range []string{
		"agents/planner/llm/default.yaml",
		"/opt/agent-core/tools/builtin/planner",
	} {
		if !strings.Contains(planner, want) {
			t.Errorf("planner request profile does not reuse canonical asset %q", want)
		}
	}
}

func TestPlannerServingFlowUsesDeclaredRemoteBoundaries(t *testing.T) {
	root := filepath.Join("..", "agents", "planner")
	machine := readServingFile(t, root, "request-machine.yaml")
	executorAt := strings.Index(machine, "action: delegate_executor")
	criticAt := strings.Index(machine, "action: delegate_critic")
	if executorAt < 0 || criticAt <= executorAt {
		t.Fatalf("planner request does not order executor before critic:\n%s", machine)
	}
	rest := readServingFile(t, root, "rest.yaml")
	for _, want := range []string{
		"${EXECUTOR_URL:-http://127.0.0.1:18210}",
		"${CRITIC_URL:-http://127.0.0.1:18220}",
		"$from(seed).parameters.task",
		"$.carried.workspace_id",
	} {
		if !strings.Contains(rest, want) {
			t.Errorf("planner serving REST contract missing %q", want)
		}
	}
	for _, forbidden := range []string{"type: exec", "--profile", "child-agent-binary"} {
		if strings.Contains(rest, forbidden) || strings.Contains(machine, forbidden) {
			t.Errorf("planner serving flow contains local child mechanism %q", forbidden)
		}
	}
}

func TestCanonicalModelDeclarationsRemainPortable(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", "catalog", "agents"))
	for _, role := range []string{"planner", "executor"} {
		declaration := readServingFile(t, root, role, "llm", "default.yaml")
		if !strings.Contains(declaration, "${OLLAMA_URL:-http://localhost:11434}") {
			t.Errorf("%s canonical declaration lacks deployment-safe endpoint default", role)
		}
	}
}

func TestServingJSONRequestHonorsExplicitDeadline(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-release
	}))
	defer server.Close()
	defer close(release)
	started := time.Now()
	_, _, err := servingJSONRequestWithTimeout(server.URL, `{}`, "", 100*time.Millisecond)
	if err == nil {
		t.Fatal("request without a response should time out")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("request deadline took %s, want under one second", elapsed)
	}
}

func readServingFile(t *testing.T, elements ...string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(elements...))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
