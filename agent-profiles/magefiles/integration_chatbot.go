// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	chatbotProfile = "agents/chatbot/profile.yaml"

	chatbotChatURL       = "http://127.0.0.1:18080/api/v1/chat"
	chatbotControlHealth = "http://127.0.0.1:18081/api/lifecycle/health"
	chatbotControlExit   = "http://127.0.0.1:18081/api/lifecycle/exit"
	chatbotMonitorState  = "http://127.0.0.1:18082/monitor/state"

	// The second rag-server (rag1) is a generated variant of the rag-server
	// profile serving a disjoint collection on a distinct port triple.
	rag1ControlHealth = "http://127.0.0.1:18096/api/lifecycle/health"
	rag1ControlExit   = "http://127.0.0.1:18096/api/lifecycle/exit"

	chromaCorpus2 = "corpus2"
)

// Chatbot proves the routed, two-RAG chat turn end to end across the mesh
// topology: one Chroma container with two disjoint collections, two rag-server
// agents (rag0 over the ingest fixture, rag1 over a seeded disjoint corpus), the
// router-enabled chatbot, and an external Ollama for the embedding, router, and
// two chat models. It drives the chat machine_request endpoint (not the browser):
//
//   - a factual turn and an analytical turn exercise the $tool router, and the
//     chatbot span log is asserted to show both the fast and the deep chat model
//     answered (the router dispatched each word);
//   - a cross-corpus turn cites chunks from both RAGs by their [rag0]/[rag1] tags
//     (sequential fan-out and distance-ordered merge);
//   - stopping rag1 degrades the next turn to a 200 answered from rag0 alone;
//   - the chatbot turn and each rag-server it called are one connected trace
//     (shared trace id, the rag-server span parented under a chatbot span).
//
// It skips (does not fail) when Docker or Ollama with the configured models is
// unavailable, naming the missing model, matching Integration.RagServer. One
// Chroma container with two collections stands in for two containers; the disjoint
// corpora are what the fan-out exercises. Cross-agent trace continuity comes from
// traceparent propagation (GH-310) and request-scoped span export (GH-362).
func (Integration) Chatbot() error {
	profilesRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := envOrDefault(agentCoreRootEnv, filepath.Join(filepath.Dir(profilesRoot), "agent-core"))
	if err := requireProfilePaths(profilesRoot,
		chatbotProfile, ragServerProfile, chromaIngestProfile,
		"agents/chatbot/rest.yaml", "agents/chatbot/request-declarations.yaml",
	); err != nil {
		return err
	}
	if reason := chatbotOllamaSkipReason(profilesRoot); reason != "" {
		fmt.Printf("SKIP chatbot: %s\n", reason)
		return nil
	}
	if _, err := exec.LookPath("docker"); err != nil {
		fmt.Println("SKIP chatbot: docker not found on PATH")
		return nil
	}
	return runChatbotIntegration(profilesRoot, coreRoot)
}

func runChatbotIntegration(profilesRoot, coreRoot string) error {
	binary, err := buildIntegrationAgent(coreRoot)
	if err != nil {
		return err
	}
	dataDir, err := os.MkdirTemp("", "agent-profiles-chatbot-data-*")
	if err != nil {
		return fmt.Errorf("create chroma data dir: %w", err)
	}
	defer os.RemoveAll(dataDir)
	containerID, err := startChromaContainer(dataDir)
	if err != nil {
		fmt.Printf("SKIP chatbot: %s\n", err)
		return nil
	}
	defer stopChromaContainer(containerID)

	// Seed rag0's collection (the ingest fixture) and rag1's disjoint collection.
	if err := runChromaIngest(binary, profilesRoot, coreRoot); err != nil {
		return err
	}
	embedModel, err := chromaEmbedModelFromConfig(profilesRoot)
	if err != nil {
		return err
	}
	if err := seedChromaCorpus2(embedModel); err != nil {
		return fmt.Errorf("seed disjoint corpus: %w", err)
	}

	// Generate the rag1 variant (distinct ports, disjoint collection).
	rag1Profile, rag1Cleanup, err := generateRag1Variant(profilesRoot)
	if err != nil {
		return err
	}
	defer rag1Cleanup()

	ragTrace0, ragCleanup0, err := chromaTraceFile("chatbot-rag0")
	if err != nil {
		return err
	}
	defer ragCleanup0()
	ragTrace1, ragCleanup1, err := chromaTraceFile("chatbot-rag1")
	if err != nil {
		return err
	}
	defer ragCleanup1()
	chatTrace, chatCleanup, err := chromaTraceFile("chatbot")
	if err != nil {
		return err
	}
	defer chatCleanup()

	stopRag0, err := startDetachedAgent(binary, profilesRoot, coreRoot, ragServerProfile, ragTrace0)
	if err != nil {
		return err
	}
	rag0Stopped := false
	defer func() {
		if !rag0Stopped {
			_ = stopRag0(true)
		}
	}()
	if err := waitHTTPStatus(ragControlHealth, http.StatusOK, 30*time.Second); err != nil {
		return fmt.Errorf("rag0 control health never came up: %w", err)
	}

	stopRag1, err := startDetachedAgent(binary, profilesRoot, coreRoot, rag1Profile, ragTrace1)
	if err != nil {
		return err
	}
	rag1Stopped := false
	defer func() {
		if !rag1Stopped {
			_ = stopRag1(true)
		}
	}()
	if err := waitHTTPStatus(rag1ControlHealth, http.StatusOK, 30*time.Second); err != nil {
		return fmt.Errorf("rag1 control health never came up: %w", err)
	}

	stopChat, err := startDetachedAgent(binary, profilesRoot, coreRoot, chatbotProfile, chatTrace)
	if err != nil {
		return err
	}
	chatStopped := false
	defer func() {
		if !chatStopped {
			_ = stopChat(true)
		}
	}()
	if err := waitHTTPStatus(chatbotControlHealth, http.StatusOK, 30*time.Second); err != nil {
		return fmt.Errorf("chatbot control health never came up: %w", err)
	}
	if err := assertChatbotMonitorReachable(); err != nil {
		return err
	}

	// Router: a factual turn and an analytical turn. The chat trace (asserted after
	// exit) must show both the fast and the deep chat model answered.
	if err := assertChatbotRoutedTurn("What do the Chroma corpus agents use to compute embeddings?"); err != nil {
		return fmt.Errorf("factual routed turn: %w", err)
	}
	if err := assertChatbotRoutedTurn("Analyze and synthesize the trade-offs across the described systems with multi-step reasoning about their relative merits."); err != nil {
		return fmt.Errorf("analytical routed turn: %w", err)
	}

	// Fan-out: a cross-corpus turn cites chunks from both RAGs by their tags.
	if err := assertChatbotFanOut(); err != nil {
		return err
	}

	// Degrade rag1: stop it gracefully (flushing its trace), then a turn still
	// returns a 200 answered from rag0 alone.
	if _, status, err := requestHTTP(http.MethodPost, rag1ControlExit, `{"reason":"chatbot integration degrade"}`); err != nil || status/100 != 2 {
		return fmt.Errorf("rag1 exit request failed: status %d: %v", status, err)
	}
	if err := stopRag1(false); err != nil {
		return fmt.Errorf("rag1 did not exit gracefully: %w", err)
	}
	rag1Stopped = true
	if err := assertChatbotDegradedTurn(); err != nil {
		return err
	}

	// Exit rag0 and the chatbot gracefully so their span logs flush.
	if _, status, err := requestHTTP(http.MethodPost, ragControlExit, `{"reason":"chatbot integration done"}`); err != nil || status/100 != 2 {
		return fmt.Errorf("rag0 exit request failed: status %d: %v", status, err)
	}
	if err := stopRag0(false); err != nil {
		return fmt.Errorf("rag0 did not exit gracefully: %w", err)
	}
	rag0Stopped = true
	if _, status, err := requestHTTP(http.MethodPost, chatbotControlExit, `{"reason":"chatbot integration done"}`); err != nil || status/100 != 2 {
		return fmt.Errorf("chatbot exit request failed: status %d: %v", status, err)
	}
	if err := stopChat(false); err != nil {
		return fmt.Errorf("chatbot did not exit gracefully: %w", err)
	}
	chatStopped = true

	fast, deep, err := chatbotAnswerModels(profilesRoot)
	if err != nil {
		return err
	}
	if err := assertChatbotRoutingTrace(chatTrace, fast, deep); err != nil {
		return err
	}
	// One connected cross-agent trace: the fan-out turn's chatbot spans and each
	// rag-server's spans share a trace id, and the rag-server's machine_request
	// span is parented under a chatbot span (traceparent propagation, GH-310/GH-362).
	if err := assertConnectedTrace(chatTrace, ragTrace0); err != nil {
		return fmt.Errorf("rag0 %w", err)
	}
	if err := assertConnectedTrace(chatTrace, ragTrace1); err != nil {
		return fmt.Errorf("rag1 %w", err)
	}

	fmt.Println("integration:chatbot PASS - router dispatched both chat models, fan-out cited both RAGs, rag1-down turn degraded to a 200, and each rag-server joined the chatbot's connected trace")
	return nil
}

// chatResponse is the shape the chatbot chat endpoint returns.
type chatResponse struct {
	Answer  string `json:"answer"`
	Error   string `json:"error"`
	Message string `json:"message"`
	Trace   struct {
		Status         string `json:"status"`
		TerminalSignal string `json:"terminal_signal"`
	} `json:"trace"`
}

func postChatTurn(message string, history string) (chatResponse, int, error) {
	body := fmt.Sprintf(`{"message":%q,"history":%s}`, message, history)
	data, status, err := requestHTTP(http.MethodPost, chatbotChatURL, body)
	if err != nil {
		return chatResponse{}, status, err
	}
	var resp chatResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return chatResponse{}, status, fmt.Errorf("decode chat response (status %d): %w: %s", status, err, data)
	}
	return resp, status, nil
}

// assertChatbotRoutedTurn drives one routed turn and asserts a grounded 200. The
// router's per-turn choice is asserted collectively from the trace afterwards.
func assertChatbotRoutedTurn(message string) error {
	resp, status, err := postChatTurn(message, "[]")
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("status = %d, want 200: %s", status, resp.Message+resp.Error)
	}
	if strings.TrimSpace(resp.Answer) == "" {
		return fmt.Errorf("empty answer")
	}
	if resp.Trace.Status != "succeeded" {
		return fmt.Errorf("trace status = %q, want succeeded", resp.Trace.Status)
	}
	return nil
}

// assertChatbotFanOut proves the sequential two-RAG fan-out: a cross-corpus turn
// cites chunks tagged from both rag0 and rag1. The chunks reach the model
// pre-tagged by rag_merge, so a grounded answer that draws on both names both tags.
func assertChatbotFanOut() error {
	resp, status, err := postChatTurn("List every project and system described across the knowledge base and cite each with its bracketed source tag.", "[]")
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("fan-out turn status = %d, want 200: %s", status, resp.Message+resp.Error)
	}
	if resp.Trace.Status != "succeeded" {
		return fmt.Errorf("fan-out turn trace status = %q, want succeeded", resp.Trace.Status)
	}
	for _, tag := range []string{"[rag0]", "[rag1]"} {
		if !strings.Contains(resp.Answer, tag) {
			return fmt.Errorf("fan-out turn answer does not cite %s (both RAGs must contribute); answer: %s", tag, resp.Answer)
		}
	}
	return nil
}

func assertChatbotDegradedTurn() error {
	// A follow-up turn carries the prior turn as client-side history (srd014 R4).
	history := `[{"role":"user","content":"What is in the knowledge base?"},{"role":"assistant","content":"Several systems and projects."}]`
	resp, status, err := postChatTurn("What do the Chroma corpus agents use to compute embeddings?", history)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("degraded turn status = %d, want 200 (rag1 down must degrade, not fail): %s", status, resp.Message+resp.Error)
	}
	if strings.TrimSpace(resp.Answer) == "" {
		return fmt.Errorf("degraded turn returned an empty answer")
	}
	if resp.Trace.Status != "succeeded" {
		return fmt.Errorf("degraded turn trace status = %q, want succeeded", resp.Trace.Status)
	}
	if strings.Contains(resp.Answer, "[rag1]") {
		return fmt.Errorf("degraded turn cited [rag1] though rag1 is stopped; answer: %s", resp.Answer)
	}
	return nil
}

func assertChatbotMonitorReachable() error {
	data, status, err := requestHTTP(http.MethodGet, chatbotMonitorState, "")
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("chatbot monitor current_state status = %d, want 200: %s", status, data)
	}
	return nil
}

// assertChatbotRoutingTrace proves, from the chatbot's own span log, that the
// router dispatched both chat-LLM words over the turns: a genai chat span for the
// fast model and one for the deep model. The genai chat spans reach the process
// OTel file; the per-word request-scoped dispatch spans run under a NoopTracer
// (agent-core internal/tools/rest/collection.go) and do not, and cross-agent
// trace continuity is the observability epic's work.
func assertChatbotRoutingTrace(tracePath, fastModel, deepModel string) error {
	spans, err := readChromaSpans(tracePath)
	if err != nil {
		return err
	}
	models := map[string]bool{}
	for _, s := range spans {
		if !strings.HasPrefix(s.Name, "chat ") {
			continue
		}
		if m, ok := s.stringAttr("gen_ai.request.model"); ok {
			models[m] = true
		}
	}
	for _, want := range []string{fastModel, deepModel} {
		if !models[want] {
			return fmt.Errorf("chatbot trace shows no chat span for model %q (router must dispatch both chat words); saw %v", want, sortedModelKeys(models))
		}
	}
	return nil
}

func sortedModelKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// assertConnectedTrace proves the chatbot turn and a rag-server it called are one
// connected cross-agent trace: their span logs share a trace id, and a rag-server
// span in that trace is parented under a chatbot span (the chatbot's outbound
// rag_query REST client span, via traceparent propagation). This replaces the
// earlier two-independent-logs assertion.
func assertConnectedTrace(chatTracePath, ragTracePath string) error {
	chatSpans, err := readChromaSpans(chatTracePath)
	if err != nil {
		return err
	}
	ragSpans, err := readChromaSpans(ragTracePath)
	if err != nil {
		return err
	}
	chatByID := map[string]string{} // spanID -> traceID, for the shared-trace check
	chatTraces := map[string]bool{}
	for _, s := range chatSpans {
		if s.SpanContext.SpanID != "" {
			chatByID[s.SpanContext.SpanID] = s.SpanContext.TraceID
		}
		if s.SpanContext.TraceID != "" {
			chatTraces[s.SpanContext.TraceID] = true
		}
	}
	shared := ""
	for _, s := range ragSpans {
		if s.SpanContext.TraceID != "" && chatTraces[s.SpanContext.TraceID] {
			shared = s.SpanContext.TraceID
			break
		}
	}
	if shared == "" {
		return fmt.Errorf("no trace id shared between the chatbot and rag-server span logs (cross-agent trace not connected)")
	}
	for _, s := range ragSpans {
		if s.SpanContext.TraceID != shared || s.Parent.SpanID == "" {
			continue
		}
		if chatByID[s.Parent.SpanID] == shared {
			return nil // a rag-server span is parented under a chatbot span in the shared trace
		}
	}
	return fmt.Errorf("shared trace %s exists but no rag-server span is parented under a chatbot span", shared)
}

var citedRecordPattern = regexp.MustCompile(`(?i)record[\s#]*([0-9]+)`)

// citedRecordNumbers extracts the 1-based record positions the model cited.
func citedRecordNumbers(answer string) []int {
	seen := map[int]bool{}
	var out []int
	for _, m := range citedRecordPattern.FindAllStringSubmatch(answer, -1) {
		n, err := strconv.Atoi(m[1])
		if err != nil || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

// disjointCorpus2Docs are the rag1 corpus, disjoint from the ingest fixture, so a
// cross-corpus turn draws chunks the ingest collection cannot supply.
var disjointCorpus2Docs = []struct{ id, text string }{
	{"solar-1", "The Solar Ridge photovoltaic array has a capacity of 55 megawatts across 140000 panels on a decommissioned quarry."},
	{"solar-2", "The Solar Ridge project feeds a 33 kilovolt substation and includes a 12 megawatt-hour battery for evening dispatch."},
}

// seedChromaCorpus2 embeds the disjoint documents at Ollama and adds them to a
// second Chroma collection, so rag1 serves a corpus disjoint from rag0's.
func seedChromaCorpus2(embedModel string) error {
	base := "http://127.0.0.1:8000/api/v2/tenants/default_tenant/databases/default_database/collections"
	data, status, err := requestHTTP(http.MethodPost, base, fmt.Sprintf(`{"name":%q,"get_or_create":true}`, chromaCorpus2))
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return fmt.Errorf("resolve %s collection: status %d: %s", chromaCorpus2, status, data)
	}
	var collection struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &collection); err != nil {
		return fmt.Errorf("decode collection id: %w", err)
	}
	ids := make([]string, 0, len(disjointCorpus2Docs))
	docs := make([]string, 0, len(disjointCorpus2Docs))
	embeddings := make([][]float64, 0, len(disjointCorpus2Docs))
	for _, d := range disjointCorpus2Docs {
		vector, err := ollamaEmbedQuery(embedModel, d.text)
		if err != nil {
			return fmt.Errorf("embed %s: %w", d.id, err)
		}
		ids = append(ids, d.id)
		docs = append(docs, d.text)
		embeddings = append(embeddings, vector)
	}
	payload, err := json.Marshal(map[string]interface{}{"ids": ids, "documents": docs, "embeddings": embeddings})
	if err != nil {
		return err
	}
	addData, addStatus, err := requestHTTP(http.MethodPost, base+"/"+collection.ID+"/add", string(payload))
	if err != nil {
		return err
	}
	if addStatus/100 != 2 {
		return fmt.Errorf("add to %s: status %d: %s", chromaCorpus2, addStatus, addData)
	}
	return nil
}

// generateRag1Variant copies the rag-server profile into a temp directory and
// rewrites its ports (18085/6/7 -> 18095/6/7) and served collection
// (corpus -> corpus2), so rag1 serves the disjoint corpus without a second
// committed profile. It returns the variant profile path and a cleanup.
func generateRag1Variant(profilesRoot string) (string, func(), error) {
	srcDir := filepath.Join(profilesRoot, "agents", "chroma", "rag-server")
	dstDir, err := os.MkdirTemp("", "agent-profiles-rag1-*")
	if err != nil {
		return "", nil, fmt.Errorf("create rag1 variant dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dstDir) }
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("read rag-server profile: %w", err)
	}
	replacer := strings.NewReplacer("18085", "18095", "18086", "18096", "18087", "18097")
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		content, err := os.ReadFile(filepath.Join(srcDir, entry.Name()))
		if err != nil {
			cleanup()
			return "", nil, err
		}
		out := replacer.Replace(string(content))
		if entry.Name() == "rest.yaml" {
			// The collection name is env-parameterized (GH-369); rewrite the default
			// so the rag1 variant serves the disjoint corpus under a local run.
			out = strings.Replace(out, "name: ${RAG_COLLECTION:-corpus}\n", "name: ${RAG_COLLECTION:-"+chromaCorpus2+"}\n", 1)
		}
		if err := os.WriteFile(filepath.Join(dstDir, entry.Name()), []byte(out), 0o644); err != nil {
			cleanup()
			return "", nil, err
		}
	}
	return filepath.Join(dstDir, "profile.yaml"), cleanup, nil
}

// chatbotOllamaSkipReason returns a non-empty reason when Ollama is unreachable or
// a model the chatbot integration needs is not installed: the chroma embed model
// that seeds both collections, the chatbot's embedding model, and the router and
// two chat models. Reading them from config keeps the gate from duplicating names.
func chatbotOllamaSkipReason(profilesRoot string) string {
	if err := waitHTTPStatus(ollamaVersionURL, http.StatusOK, 2*time.Second); err != nil {
		return fmt.Sprintf("Ollama not reachable at %s: %v", ollamaVersionURL, err)
	}
	required, err := chatbotRequiredModels(profilesRoot)
	if err != nil {
		return fmt.Sprintf("read chatbot model config: %v", err)
	}
	names, err := fetchChromaOllamaModels()
	if err != nil {
		return fmt.Sprintf("Ollama /api/tags preflight failed: %v", err)
	}
	for _, model := range required {
		if !chromaModelInstalled(names, model) {
			return fmt.Sprintf("Ollama model %q not installed; available: %s", model, strings.Join(names, ", "))
		}
	}
	return ""
}

func chatbotRequiredModels(profilesRoot string) ([]string, error) {
	set := map[string]bool{}
	ingestEmbed, err := chromaEmbedModelFromConfig(profilesRoot)
	if err != nil {
		return nil, err
	}
	set[ingestEmbed] = true
	embed, err := chatbotEmbedModelFromConfig(profilesRoot)
	if err != nil {
		return nil, err
	}
	set[embed] = true
	chatModels, err := chatbotChatModels(profilesRoot)
	if err != nil {
		return nil, err
	}
	for _, model := range chatModels {
		set[model] = true
	}
	models := make([]string, 0, len(set))
	for model := range set {
		models = append(models, model)
	}
	sort.Strings(models)
	return models, nil
}

// chatbotEmbedModelFromConfig reads the embedding model from the embedding client
// embed_query operation in agents/chatbot/rest.yaml.
func chatbotEmbedModelFromConfig(profilesRoot string) (string, error) {
	var cfg struct {
		Rest struct {
			Clients map[string]struct {
				Operations map[string]struct {
					Body struct {
						Model string `yaml:"model"`
					} `yaml:"body"`
				} `yaml:"operations"`
			} `yaml:"clients"`
		} `yaml:"rest"`
	}
	path := filepath.Join(profilesRoot, "agents", "chatbot", "rest.yaml")
	if err := readIntegrationYAML(path, "chatbot rest asset", &cfg); err != nil {
		return "", err
	}
	model := cfg.Rest.Clients["embedding"].Operations["embed_query"].Body.Model
	if model == "" {
		return "", fmt.Errorf("no embedding model in %s", path)
	}
	return model, nil
}

// chatbotToolModels reads the model of every invoke_llm-init word in the chatbot's
// request-declarations.yaml keyed by word name (route, invoke_llm_fast, invoke_llm_deep).
func chatbotToolModels(profilesRoot string) (map[string]string, error) {
	var cfg struct {
		Tools []struct {
			Name   string `yaml:"name"`
			Init   string `yaml:"init"`
			Config struct {
				Model string `yaml:"model"`
			} `yaml:"config"`
		} `yaml:"tools"`
	}
	path := filepath.Join(profilesRoot, "agents", "chatbot", "request-declarations.yaml")
	if err := readIntegrationYAML(path, "chatbot request declarations", &cfg); err != nil {
		return nil, err
	}
	models := map[string]string{}
	for _, tool := range cfg.Tools {
		if tool.Init == "invoke_llm" && tool.Config.Model != "" {
			models[tool.Name] = tool.Config.Model
		}
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("no invoke_llm word with a model in %s", path)
	}
	return models, nil
}

// chatbotChatModels returns the distinct models of the chatbot's invoke_llm words.
func chatbotChatModels(profilesRoot string) ([]string, error) {
	models, err := chatbotToolModels(profilesRoot)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(models))
	for _, model := range models {
		out = append(out, model)
	}
	sort.Strings(out)
	return out, nil
}

// chatbotAnswerModels returns the fast and deep chat-LLM answer word models.
func chatbotAnswerModels(profilesRoot string) (fast, deep string, err error) {
	models, err := chatbotToolModels(profilesRoot)
	if err != nil {
		return "", "", err
	}
	fast, okFast := models["invoke_llm_fast"]
	deep, okDeep := models["invoke_llm_deep"]
	if !okFast || !okDeep {
		return "", "", fmt.Errorf("chatbot must declare invoke_llm_fast and invoke_llm_deep; got %v", models)
	}
	return fast, deep, nil
}
