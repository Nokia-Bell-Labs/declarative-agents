// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package conformance

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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
	Init       string         `yaml:"init"`
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
		!containsKnowledgeState(machine.States, "ListingCorpus") ||
		!containsKnowledgeState(machine.States, "NormalizingEmbedding") {
		t.Fatal("canonical corpus-ingest machine does not expose discovery, listing, and embedding-normalization states")
	}
	requireKnowledgeTransition(t, machine.Transitions,
		"CheckingOllama", "OllamaReady", "DiscoveringCorpus", "list_resource_paths")
	requireKnowledgeTransition(t, machine.Transitions,
		"DiscoveringCorpus", "ToolDone", "ListingCorpus", "list_resource")
	requireKnowledgeTransition(t, machine.Transitions,
		"ListingCorpus", "DocumentListReady", "Composing", "invoke_llm")
	requireKnowledgeTransition(t, machine.Transitions,
		"Composing", "DocumentMissing", "Composing", "invoke_llm")
	requireKnowledgeTransition(t, machine.Transitions,
		"Composing", "DocumentResourceDenied", "Composing", "invoke_llm")
	requireKnowledgeTransition(t, machine.Transitions,
		"Embedding", "DocumentEmbedded", "NormalizingEmbedding", "normalize_embedding")
	requireKnowledgeTransition(t, machine.Transitions,
		"NormalizingEmbedding", "EmbeddingNormalized", "ResolvingCollection", "resolve_collection")
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
	normalize := knowledgeTool(t, declarations.Tools, "normalize_embedding")
	if normalize.Init != "normalize_vector" || normalize.Visibility != "internal" ||
		normalize.Config["path"] != "mapped.embedding" {
		t.Fatalf("normalize_embedding binding = init %q visibility %q config %#v",
			normalize.Init, normalize.Visibility, normalize.Config)
	}
}

func TestCorpusIngestSelectsConfiguredEmbeddingProvider(t *testing.T) {
	t.Parallel()
	ingestRoot := ProfilePath(filepath.Join("agents", "knowledge-manager", "corpus-ingest"))
	restPath := ProfilePath(filepath.Join("agents", "knowledge-manager", "corpus-rest.yaml"))
	restData, err := os.ReadFile(restPath)
	if err != nil {
		t.Fatalf("read corpus REST definition: %v", err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(restData, &document); err != nil {
		t.Fatalf("parse corpus REST definition: %v", err)
	}
	rest := knowledgeMap(t, document["rest"], "rest")
	clients := knowledgeMap(t, rest["clients"], "rest.clients")
	ollama := knowledgeMap(t, clients["ollama"], "rest.clients.ollama")
	ollamaOperations := knowledgeMap(t, ollama["operations"], "rest.clients.ollama.operations")
	documentEmbed := knowledgeMap(t, ollamaOperations["embed"], "ollama.embed")
	queryEmbed := knowledgeMap(t, ollamaOperations["embed_query"], "ollama.embed_query")
	delete(ollamaOperations, "embed")
	delete(ollamaOperations, "embed_query")

	const sharedModel = "${COHERE_EMBEDDING_MODEL:-embed-v4.0}"
	documentEmbed["path"] = "/v2/embed"
	documentEmbed["body"] = map[string]any{
		"model": sharedModel, "input_type": "search_document",
		"texts": []string{"{{ params.input }}"},
	}
	queryEmbed["path"] = "/v2/embed"
	queryEmbed["body"] = map[string]any{
		"model": sharedModel, "input_type": "search_query",
		"texts": []string{"What does this corpus describe?"},
	}
	clients["cohere"] = map[string]any{
		"base_url":   "http://127.0.0.1:11434",
		"auth_ref":   "none",
		"limits_ref": "local_corpus",
		"operations": map[string]any{
			"embed_document_cohere": documentEmbed,
			"embed_query_cohere":    queryEmbed,
		},
	}

	providerREST, err := yaml.Marshal(document)
	if err != nil {
		t.Fatalf("marshal provider REST definition: %v", err)
	}
	dir := t.TempDir()
	providerRESTPath := writeEphemeral(t, dir, "corpus-rest.yaml", string(providerREST))
	profile := writeEphemeral(t, dir, "profile.yaml", fmt.Sprintf(`name: corpus-ingest-provider
machine: %q
tools: [%q]
tool_declarations:
  - /opt/agent-core/tools/builtin/llm/all.yaml
  - %q
rest_definitions: [%q]
`, filepath.Join(ingestRoot, "machine.yaml"), filepath.Join(ingestRoot, "tools.yaml"),
		filepath.Join(ingestRoot, "declarations.yaml"), providerRESTPath))

	result := Run(t, RunConfig{
		Profile: profile,
		Args:    []string{"--validate-config"},
		Env: []string{
			"CORPUS_INGEST_EMBEDDING_REST_REF=cohere",
			"CORPUS_INGEST_EMBEDDING_OPERATION=embed_document_cohere",
		},
	})
	if result.ExitCode != 0 {
		t.Fatalf("configured provider profile did not validate:\n%s", result.Output)
	}
}

func TestCorpusIngestNormalizesProviderEmbeddings(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, embeddingResponse string
		cohere, wantAdded       bool
		wantExit                int
		wantTerminal            string
	}{
		{
			name: "Ollama flat vector", embeddingResponse: `{"embedding":[0.25,0.75]}`,
			wantAdded: true, wantExit: 0, wantTerminal: "Succeeded",
		},
		{
			name: "Cohere single row", embeddingResponse: `{"embeddings":{"float":[[0.25,0.75]]}}`,
			cohere: true, wantAdded: true, wantExit: 0, wantTerminal: "Succeeded",
		},
		{
			name: "multiple rows fail before add", embeddingResponse: `{"embeddings":{"float":[[0.25],[0.75]]}}`,
			cohere: true, wantExit: 2, wantTerminal: "Failed",
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var chatCalls atomic.Int32
			var addMu sync.Mutex
			var addBody string
			fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/tags":
					writeKnowledgeJSON(t, w, map[string]any{"models": []any{
						map[string]any{"name": "ornith:9b"},
					}})
				case r.Method == http.MethodGet && r.URL.Path == "/api/version":
					writeKnowledgeJSON(t, w, map[string]any{"version": "conformance"})
				case r.Method == http.MethodGet && r.URL.Path == "/api/v2/heartbeat":
					writeKnowledgeJSON(t, w, map[string]any{"nanosecond heartbeat": 1})
				case r.Method == http.MethodPost &&
					(r.URL.Path == "/api/embeddings" || r.URL.Path == "/v2/embed"):
					_, _ = w.Write([]byte(tc.embeddingResponse))
				case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/collections"):
					writeKnowledgeJSON(t, w, map[string]any{
						"id": "collection-1", "name": "corpus",
					})
				case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/collections/collection-1/add"):
					body := readKnowledgeBody(t, r)
					addMu.Lock()
					addBody = body
					addMu.Unlock()
					writeKnowledgeJSON(t, w, map[string]any{})
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/collections/collection-1/count"):
					_, _ = w.Write([]byte(`1`))
				case r.Method == http.MethodPost && r.URL.Path == "/api/chat":
					content := `[tool_call]
{"tool":"done","parameters":{"summary":"Ingested 1 corpus document."}}
[/tool_call]`
					if chatCalls.Add(1) == 1 {
						content = `[tool_call]
{"tool":"read_resource","parameters":{"resource":"corpus","path":"doc.md"}}
[/tool_call]`
					}
					writeKnowledgeJSON(t, w, map[string]any{
						"message":    map[string]any{"role": "assistant", "content": content},
						"eval_count": 8, "prompt_eval_count": 16,
					})
				default:
					http.NotFound(w, r)
				}
			}))
			defer fixture.Close()

			workspace := t.TempDir()
			if err := os.WriteFile(
				filepath.Join(workspace, "doc.md"), []byte("provider document\n"), 0o644,
			); err != nil {
				t.Fatal(err)
			}
			profile := corpusIngestProviderProfile(t, fixture.URL, tc.cohere)
			env := []string{"CORPUS_CHAT_MODEL=ornith:9b"}
			if tc.cohere {
				env = append(env,
					"CORPUS_INGEST_EMBEDDING_REST_REF=cohere",
					"CORPUS_INGEST_EMBEDDING_OPERATION=embed_document_cohere",
				)
			}
			result := Run(t, RunConfig{
				Profile: profile, Directory: workspace, Env: env, Timeout: 30 * time.Second,
			})

			result.RequireExit(t, tc.wantExit)
			result.RootRequired(t)
			result.RequireTerminalState(t, tc.wantTerminal)
			result.RequireToolSpans(t, "embed_document", "normalize_embedding")
			addMu.Lock()
			gotAddBody := addBody
			addMu.Unlock()
			if !tc.wantAdded {
				if gotAddBody != "" {
					t.Fatalf("invalid embedding reached Chroma add: %s", gotAddBody)
				}
				return
			}
			result.RequireNoErrorSpans(t)
			result.RequireToolSpans(t, "resolve_collection", "chroma_add", "chroma_count")
			var add struct {
				Embeddings [][]float64 `json:"embeddings"`
				Documents  []string    `json:"documents"`
				IDs        []string    `json:"ids"`
			}
			if err := json.Unmarshal([]byte(gotAddBody), &add); err != nil {
				t.Fatalf("decode Chroma add body %q: %v", gotAddBody, err)
			}
			if len(add.Embeddings) != 1 || len(add.Embeddings[0]) != 2 ||
				add.Embeddings[0][0] != 0.25 || add.Embeddings[0][1] != 0.75 {
				t.Fatalf("Chroma embeddings = %#v, want exactly [[0.25,0.75]]", add.Embeddings)
			}
			if len(add.Documents) != 1 || add.Documents[0] != "provider document\n" {
				t.Fatalf("Chroma documents = %#v, want original document", add.Documents)
			}
			if len(add.IDs) != 1 || add.IDs[0] != "doc.md" {
				t.Fatalf("Chroma ids = %#v, want doc.md", add.IDs)
			}
		})
	}
}

func corpusIngestProviderProfile(t *testing.T, fixtureURL string, cohere bool) string {
	t.Helper()
	restData, err := os.ReadFile(ProfilePath(
		filepath.Join("agents", "knowledge-manager", "corpus-rest.yaml"),
	))
	if err != nil {
		t.Fatalf("read corpus REST definition: %v", err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(restData, &document); err != nil {
		t.Fatalf("parse corpus REST definition: %v", err)
	}
	rest := knowledgeMap(t, document["rest"], "rest")
	clients := knowledgeMap(t, rest["clients"], "rest.clients")
	ollama := knowledgeMap(t, clients["ollama"], "rest.clients.ollama")
	chroma := knowledgeMap(t, clients["chroma"], "rest.clients.chroma")
	ollama["base_url"] = fixtureURL
	chroma["base_url"] = fixtureURL
	parsedURL, err := url.Parse(fixtureURL)
	if err != nil {
		t.Fatalf("parse fixture URL: %v", err)
	}
	port, err := strconv.Atoi(parsedURL.Port())
	if err != nil {
		t.Fatalf("parse fixture port: %v", err)
	}
	limits := knowledgeMap(t, rest["limits"], "rest.limits")
	localCorpus := knowledgeMap(t, limits["local_corpus"], "rest.limits.local_corpus")
	network := knowledgeMap(t, localCorpus["network"], "rest.limits.local_corpus.network")
	network["ports"] = []int{port}

	if cohere {
		operations := knowledgeMap(t, ollama["operations"], "rest.clients.ollama.operations")
		encoded, err := yaml.Marshal(knowledgeMap(t, operations["embed"], "ollama.embed"))
		if err != nil {
			t.Fatalf("copy Ollama embed operation: %v", err)
		}
		var embed map[string]any
		if err := yaml.Unmarshal(encoded, &embed); err != nil {
			t.Fatalf("decode copied embed operation: %v", err)
		}
		embed["path"] = "/v2/embed"
		embed["body"] = map[string]any{
			"model":      "${COHERE_EMBEDDING_MODEL:-embed-v4.0}",
			"input_type": "search_document",
			"texts":      []string{"{{ params.input }}"},
		}
		response := knowledgeMap(t, embed["response"], "cohere.embed.response")
		response["output"] = map[string]any{"embedding": "$.embeddings.float"}
		clients["cohere"] = map[string]any{
			"base_url": fixtureURL, "auth_ref": "none", "limits_ref": "local_corpus",
			"operations": map[string]any{"embed_document_cohere": embed},
		}
	}

	providerREST, err := yaml.Marshal(document)
	if err != nil {
		t.Fatalf("marshal provider REST definition: %v", err)
	}
	dir := t.TempDir()
	restPath := writeEphemeral(t, dir, "corpus-rest.yaml", string(providerREST))
	ingestRoot := ProfilePath(filepath.Join("agents", "knowledge-manager", "corpus-ingest"))
	return writeEphemeral(t, dir, "profile.yaml", fmt.Sprintf(`name: corpus-ingest-provider
machine: %q
tools: [%q]
tool_declarations:
  - /opt/agent-core/tools/builtin/llm/all.yaml
  - %q
rest_definitions: [%q]
`, filepath.Join(ingestRoot, "machine.yaml"), filepath.Join(ingestRoot, "tools.yaml"),
		filepath.Join(ingestRoot, "declarations.yaml"), restPath))
}

func writeKnowledgeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Errorf("write fixture response: %v", err)
	}
}

func knowledgeMap(t *testing.T, value any, path string) map[string]any {
	t.Helper()
	mapped, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s = %T, want map", path, value)
	}
	return mapped
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
