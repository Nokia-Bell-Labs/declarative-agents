// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// TestKnowledgeManagerConformance launches the documentation-curator profile,
// waits for its documentation host to become healthy, posts the control
// lifecycle exit, and asserts the machine stops its owned listeners and reaches
// the Done terminal state.
//
// This is the heaviest server family: the profile binds a generic curator HTTP
// server, a control REST server, and a monitor REST server. It
// runs the wrapper an operator ships —
// agents/knowledge-manager/documentation-curator/profile.yaml — through a temp
// copy of its whole directory tree (so machine.yaml, tools.yaml, the tool and
// REST declarations, request-machine.yaml, openapi.yaml, and the ui/ assets all
// resolve from the copy), patching only the three bound addresses. The
// /opt/agent-core declarations remap onto the checkout via --core-root.
//
// Traces srd011-knowledge-manager: R2.2 (documentation-serving and lifecycle
// control tool families), R3.1 (control exit and listener shutdown), and R3.2
// (Done terminal outcome).
func TestKnowledgeManagerConformance(t *testing.T) {
	t.Parallel()
	coreRoot := RequireCoreRoot(t)

	docsAddr := FreeAddr(t)
	controlAddr := FreeAddr(t)
	monitorAddr := FreeAddr(t)

	profilePath := CopyShippedProfile(t,
		filepath.Join("agents", "knowledge-manager", "documentation-curator", "profile.yaml"),
		map[string]string{
			// Bind the three generic REST servers to free ports.
			"http://127.0.0.1:18081":   "http://" + docsAddr,
			"ports: [18081]":           "ports: [" + PortOf(t, docsAddr) + "]",
			"ports: [18082]":           "ports: [" + PortOf(t, controlAddr) + "]",
			"ports: [18084]":           "ports: [" + PortOf(t, monitorAddr) + "]",
			"address: 127.0.0.1:18081": "address: " + docsAddr,
			"address: 127.0.0.1:18082": "address: " + controlAddr,
			"address: 127.0.0.1:18084": "address: " + monitorAddr,
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
		"launch_curator_http",
		"launch_curator_control",
		"launch_monitor_rest",
		"await_curator_control",
		"exit_agent",
		"stop_monitor_rest",
		"stop_curator_http",
	)

	// srd011 R3.2: the machine reaches the Done terminal state.
	result.RequireTerminalState(t, "Done")
}

// TestCorpusReaderConformance executes the shipped reader wrapper against
// deterministic Chroma and Ollama protocol fixtures. Only transport addresses
// are patched; the shipped machine, declarations, REST operations, prompt, and
// tool selection remain in control of sequencing and data threading.
func TestCorpusReaderConformance(t *testing.T) {
	t.Parallel()
	var embedded, queried, grounded atomic.Bool
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"ornith:9b"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/version":
			_, _ = w.Write([]byte(`{"version":"conformance"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/heartbeat":
			_, _ = w.Write([]byte(`{"nanosecond heartbeat":1}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/embeddings":
			body := readKnowledgeBody(t, r)
			if !strings.Contains(body, "What does this corpus describe?") {
				t.Errorf("embedding request does not contain shipped question: %s", body)
			}
			embedded.Store(true)
			_, _ = w.Write([]byte(`{"embedding":[0.25,0.75]}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/collections"):
			_, _ = w.Write([]byte(`{"id":"collection-1","name":"corpus"}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/collections/collection-1/query"):
			body := readKnowledgeBody(t, r)
			if !strings.Contains(body, "0.25") || !strings.Contains(body, "0.75") {
				t.Errorf("query request does not contain provider embedding: %s", body)
			}
			queried.Store(true)
			_, _ = w.Write([]byte(`{"ids":[["chunk-alpha"]],"documents":[["The corpus describes declarative agents."]]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/chat":
			body := readKnowledgeBody(t, r)
			if !strings.Contains(body, "chunk-alpha") || !strings.Contains(body, "declarative agents") {
				t.Errorf("chat request is not grounded in retrieved chunk: %s", body)
			}
			grounded.Store(true)
			_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"The corpus describes declarative agents [chunk-alpha]."},"eval_count":8,"prompt_eval_count":16}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer fixture.Close()

	profile := copyCorpusReaderProfile(t, fixture.URL)
	result := Run(t, RunConfig{Profile: profile, Directory: t.TempDir()})

	result.RequireExit(t, 0)
	result.RootRequired(t)
	result.RequireNoErrorSpans(t)
	result.RequireToolSpans(t,
		"chroma_ready",
		"ollama_ready",
		"embed_query",
		"resolve_collection",
		"chroma_query",
	)
	if got := len(result.Spans.Named("chat ornith:9b")); got == 0 {
		t.Fatalf("missing invoke_llm chat span; span names: %v", result.Spans.Names())
	}
	result.RequireTerminalState(t, "Succeeded")
	if !embedded.Load() || !queried.Load() || !grounded.Load() {
		t.Fatalf("reader boundary observations: embedded=%t queried=%t grounded=%t",
			embedded.Load(), queried.Load(), grounded.Load())
	}
}

func TestCorpusReaderRESTFailureConformance(t *testing.T) {
	t.Parallel()
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/tags" {
			_, _ = w.Write([]byte(`{"models":[{"name":"ornith:9b"}]}`))
			return
		}
		if r.URL.Path == "/api/v2/heartbeat" {
			http.Error(w, `{"error":"unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		http.NotFound(w, r)
	}))
	defer fixture.Close()

	result := Run(t, RunConfig{
		Profile:   copyCorpusReaderProfile(t, fixture.URL),
		Directory: t.TempDir(),
	})

	result.RequireExit(t, 2)
	result.RootRequired(t)
	result.RequireToolSpans(t, "chroma_ready")
	result.RequireTerminalState(t, "Failed")
	if got := len(result.Spans.Named("execute_tool embed_query")); got != 0 {
		t.Fatalf("embed_query ran after failed Chroma readiness: %d spans", got)
	}
}

func copyCorpusReaderProfile(t *testing.T, fixtureURL string) string {
	t.Helper()
	profile := CopyShippedProfile(t,
		filepath.Join("agents", "knowledge-manager", "corpus-reader", "profile.yaml"),
		map[string]string{
			"../corpus-rest.yaml":    "corpus-rest.yaml",
			"http://localhost:11434": fixtureURL,
			"http://127.0.0.1:11434": fixtureURL,
		})
	restData, err := os.ReadFile(ProfilePath(filepath.Join("agents", "knowledge-manager", "corpus-rest.yaml")))
	if err != nil {
		t.Fatalf("read shipped corpus REST definition: %v", err)
	}
	parsed, err := url.Parse(fixtureURL)
	if err != nil {
		t.Fatalf("parse fixture URL: %v", err)
	}
	rest := strings.ReplaceAll(string(restData), "http://127.0.0.1:11434", fixtureURL)
	rest = strings.ReplaceAll(rest, "http://127.0.0.1:8000", fixtureURL)
	rest = strings.ReplaceAll(rest, "ports: [8000, 11434]", "ports: ["+parsed.Port()+"]")
	if err := os.WriteFile(filepath.Join(filepath.Dir(profile), "corpus-rest.yaml"), []byte(rest), 0o644); err != nil {
		t.Fatalf("write patched corpus REST definition: %v", err)
	}
	return profile
}

func readKnowledgeBody(t *testing.T, r *http.Request) string {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Errorf("read %s request body: %v", r.URL.Path, err)
	}
	return string(body)
}

type knowledgeToolDeclaration struct {
	Name       string         `yaml:"name"`
	Type       string         `yaml:"type"`
	Binary     string         `yaml:"binary"`
	Visibility string         `yaml:"visibility"`
	Config     map[string]any `yaml:"config"`
}

func TestCorpusIngestListsTrustedCorpusBeforeModelControl(t *testing.T) {
	t.Parallel()
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
	if !containsKnowledgeState(machine.States, "DiscoveringCorpus") ||
		!containsKnowledgeState(machine.States, "ListingCorpus") {
		t.Fatal("canonical corpus-ingest machine does not expose discovery and shaping states")
	}
	requireKnowledgeTransition(t, machine.Transitions,
		"CheckingOllama", "OllamaReady", "DiscoveringCorpus", "list_resource_paths")
	requireKnowledgeTransition(t, machine.Transitions,
		"DiscoveringCorpus", "ToolDone", "ListingCorpus", "list_resource")
	requireKnowledgeTransition(t, machine.Transitions,
		"ListingCorpus", "DocumentListReady", "Composing", "invoke_llm")
	for _, transition := range machine.Transitions {
		if transition.State == "Composing" && transition.Signal == "DocumentListReady" {
			t.Fatal("model-controlled Composing state still owns corpus discovery")
		}
	}

	var declarations struct {
		Tools []knowledgeToolDeclaration `yaml:"tools"`
	}
	path := filepath.Join("..", "agents", "knowledge-manager", "corpus-ingest", "declarations.yaml")
	readKnowledgeYAML(t, path, &declarations)
	list := knowledgeTool(t, declarations.Tools, "list_resource")
	if list.Visibility != "internal" || list.Config["resource"] != "corpus" {
		t.Fatalf("list_resource authority = visibility %q config %#v", list.Visibility, list.Config)
	}
	paths := knowledgeTool(t, declarations.Tools, "list_resource_paths")
	if paths.Type != "exec" || paths.Binary != "bash" || paths.Visibility != "internal" {
		t.Fatalf("list_resource_paths boundary = type %q binary %q visibility %q",
			paths.Type, paths.Binary, paths.Visibility)
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

func knowledgeTool(
	t *testing.T,
	tools []knowledgeToolDeclaration,
	name string,
) knowledgeToolDeclaration {
	t.Helper()
	for _, tool := range tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("missing tool %s", name)
	return knowledgeToolDeclaration{}
}
