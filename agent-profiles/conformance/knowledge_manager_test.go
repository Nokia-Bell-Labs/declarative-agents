// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// TestKnowledgeManagerConformance launches the documentation-curator profile,
// waits for its documentation host to become healthy, posts the control
// lifecycle exit, and asserts the machine stops its owned listeners and reaches
// the Done terminal state.
//
// This is the heaviest server family: the profile binds a documentation host, a
// control REST server, a machine-request server, and a monitor REST server. It
// runs the wrapper an operator ships —
// agents/knowledge-manager/documentation-curator/profile.yaml — through a temp
// copy of its whole directory tree (so machine.yaml, tools.yaml, the tool and
// REST declarations, request-machine.yaml, openapi.yaml, and the ui/ assets all
// resolve from the copy), patching only the four bound addresses and the docs
// host's corpus directories. The address patches are required so the four
// listeners take free loopback ports; the corpus directories (docs_dir,
// configs_dir, source_dir) are patched to the checkout because the shipped host
// resolves them relative to its own working directory and would otherwise not
// find a docs corpus to serve. The /opt/agent-core exit-agent declaration remaps
// onto the checkout via --core-root.
//
// Traces srd011-knowledge-manager: R2.2 (documentation-serving and lifecycle
// control tool families), R3.1 (control exit and listener shutdown), and R3.2
// (Done terminal outcome).
func TestKnowledgeManagerConformance(t *testing.T) {
	coreRoot := RequireCoreRoot(t)

	docsAddr := FreeAddr(t)
	controlAddr := FreeAddr(t)
	requestAddr := FreeAddr(t)
	monitorAddr := FreeAddr(t)

	profilePath := CopyShippedProfile(t,
		filepath.Join("agents", "knowledge-manager", "documentation-curator", "profile.yaml"),
		map[string]string{
			// Bind the documentation host and the three REST servers to free ports.
			"addr: :18081":             `addr: "` + docsAddr + `"`,
			"http://127.0.0.1:18081":   "http://" + docsAddr,
			"ports: [18081]":           "ports: [" + PortOf(t, docsAddr) + "]",
			"ports: [18082]":           "ports: [" + PortOf(t, controlAddr) + "]",
			"ports: [18083]":           "ports: [" + PortOf(t, requestAddr) + "]",
			"ports: [18084]":           "ports: [" + PortOf(t, monitorAddr) + "]",
			"address: 127.0.0.1:18082": "address: " + controlAddr,
			"address: 127.0.0.1:18083": "address: " + requestAddr,
			"address: 127.0.0.1:18084": "address: " + monitorAddr,
			// Point the docs host at the checkout's corpus so it serves without
			// depending on the process working directory.
			"docs_dir: docs":       "docs_dir: " + fmt.Sprintf("%q", filepath.Join(coreRoot, "docs")),
			"configs_dir: configs": "configs_dir: " + fmt.Sprintf("%q", filepath.Join(coreRoot, "configs")),
			"source_dir: .":        "source_dir: " + fmt.Sprintf("%q", coreRoot),
		})

	server := Serve(t, ServeConfig{Profile: profilePath, Directory: coreRoot})
	server.WaitHealthy("http://"+docsAddr+"/api/v1/health", 15*time.Second)
	if status := server.Post("http://"+controlAddr+"/api/lifecycle/exit", `{"reason":"conformance","status":"success"}`); status != http.StatusAccepted {
		t.Fatalf("lifecycle exit POST status = %d, want %d", status, http.StatusAccepted)
	}
	result := server.WaitExit(15 * time.Second)

	// srd011 R3.2: clean terminal outcome with no error-status spans.
	result.RequireExit(t, 0)
	result.RootRequired(t)
	result.RequireNoErrorSpans(t)

	// srd011 R2.2/R3.1: documentation host, control launch/await, monitor
	// launch/stop, and exit_agent lifecycle vocabulary is visible.
	result.RequireToolSpans(t,
		"launch_documentation",
		"launch_curator_control",
		"launch_monitor_rest",
		"await_curator_control",
		"exit_agent",
		"stop_monitor_rest",
		"stop_documentation",
	)

	// srd011 R3.2: the machine reaches the Done terminal state.
	result.RequireTerminalState(t, "Done")
}

func TestCorpusIngestListsTrustedCorpusBeforeModelControl(t *testing.T) {
	var machine struct {
		States []struct {
			Name string `yaml:"name"`
		} `yaml:"states"`
		Transitions []struct {
			State  string `yaml:"state"`
			Signal string `yaml:"signal"`
			Next   string `yaml:"next"`
			Action string `yaml:"action"`
		} `yaml:"transitions"`
	}
	readKnowledgeYAML(t,
		filepath.Join("..", "agents", "knowledge-manager", "corpus-ingest", "machine.yaml"),
		&machine)
	if !containsKnowledgeState(machine.States, "ListingCorpus") {
		t.Fatal("canonical corpus-ingest machine has no ListingCorpus state")
	}
	requireKnowledgeTransition(t, machine.Transitions,
		"CheckingOllama", "OllamaReady", "ListingCorpus", "list_resource")
	requireKnowledgeTransition(t, machine.Transitions,
		"ListingCorpus", "DocumentListReady", "Composing", "invoke_llm")
	for _, transition := range machine.Transitions {
		if transition.State == "Composing" && transition.Signal == "DocumentListReady" {
			t.Fatal("model-controlled Composing state still owns corpus discovery")
		}
	}

	var declarations struct {
		Tools []struct {
			Name       string         `yaml:"name"`
			Visibility string         `yaml:"visibility"`
			Config     map[string]any `yaml:"config"`
		} `yaml:"tools"`
	}
	path := filepath.Join("..", "agents", "knowledge-manager", "corpus-ingest", "declarations.yaml")
	readKnowledgeYAML(t, path, &declarations)
	list := knowledgeTool(t, declarations.Tools, "list_resource")
	if list.Visibility != "internal" || list.Config["resource"] != "corpus" {
		t.Fatalf("list_resource authority = visibility %q config %#v", list.Visibility, list.Config)
	}
	invoke := knowledgeTool(t, declarations.Tools, "invoke_llm")
	model, _ := invoke.Config["model"].(string)
	provider, _ := invoke.Config["provider_url"].(string)
	if !strings.Contains(model, "CORPUS_CHAT_MODEL") ||
		!strings.Contains(provider, "OLLAMA_URL") {
		t.Fatalf("canonical model parameterization = model %q provider %q", model, provider)
	}
}

func readKnowledgeYAML(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(data, target); err != nil {
		t.Fatal(err)
	}
}

func containsKnowledgeState(states []struct {
	Name string `yaml:"name"`
}, name string) bool {
	for _, state := range states {
		if state.Name == name {
			return true
		}
	}
	return false
}

func requireKnowledgeTransition(t *testing.T, transitions []struct {
	State  string `yaml:"state"`
	Signal string `yaml:"signal"`
	Next   string `yaml:"next"`
	Action string `yaml:"action"`
}, state, signal, next, action string) {
	t.Helper()
	for _, transition := range transitions {
		if transition.State == state && transition.Signal == signal &&
			transition.Next == next && transition.Action == action {
			return
		}
	}
	t.Fatalf("missing transition %s/%s -> %s action %s", state, signal, next, action)
}

func knowledgeTool(t *testing.T, tools []struct {
	Name       string         `yaml:"name"`
	Visibility string         `yaml:"visibility"`
	Config     map[string]any `yaml:"config"`
}, name string) struct {
	Name       string         `yaml:"name"`
	Visibility string         `yaml:"visibility"`
	Config     map[string]any `yaml:"config"`
} {
	t.Helper()
	for _, tool := range tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("missing tool %s", name)
	return struct {
		Name       string         `yaml:"name"`
		Visibility string         `yaml:"visibility"`
		Config     map[string]any `yaml:"config"`
	}{}
}
