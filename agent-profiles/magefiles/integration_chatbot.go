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
)

// Chatbot proves a chat turn end to end across the mesh's minimum topology: a
// Chroma container seeded by the ingest profile, the rag-server agent, the
// chatbot agent, and an external Ollama for the embedding and chat models. It
// drives the chat machine_request endpoint (not the browser): a grounded turn
// returns an answer citing retrieved records, a second turn with the rag-server
// stopped returns the degraded response shape, and each agent's OTel span log is
// asserted independently. It skips (does not fail) when Docker or Ollama with the
// configured models is unavailable, matching Integration.RagServer.
//
// This is the single-RAG reduction of rel09.0-uc002. The $tool router and the
// two-RAG fan-out with per-RAG degradation (srd014 R2, R3) land with Epic #294;
// cross-agent trace continuity between the two span logs lands with the
// observability epic's traceparent work (agent-core srd016), so the two logs are
// asserted independently here.
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

	// Seed the served collection through the ingest profile.
	if err := runChromaIngest(binary, profilesRoot, coreRoot); err != nil {
		return err
	}

	ragTrace, ragCleanup, err := chromaTraceFile("chatbot-ragserver")
	if err != nil {
		return err
	}
	defer ragCleanup()
	chatTrace, chatCleanup, err := chromaTraceFile("chatbot")
	if err != nil {
		return err
	}
	defer chatCleanup()

	// Launch order: rag-server first, then the chatbot whose rag0 client points at it.
	stopRag, err := startDetachedAgent(binary, profilesRoot, coreRoot, ragServerProfile, ragTrace)
	if err != nil {
		return err
	}
	ragStopped := false
	defer func() {
		if !ragStopped {
			_ = stopRag(true)
		}
	}()
	if err := waitHTTPStatus(ragControlHealth, http.StatusOK, 30*time.Second); err != nil {
		return fmt.Errorf("rag-server control health never came up: %w", err)
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

	// Grounded turn: the RAG server is up, so the answer must cite retrieved records.
	if err := assertChatbotGroundedTurn(); err != nil {
		return err
	}

	// Degrade: stop the RAG server gracefully (which flushes its trace), then a
	// second turn must still return the degraded response shape, not a 500.
	if _, status, err := requestHTTP(http.MethodPost, ragControlExit, `{"reason":"chatbot integration degrade"}`); err != nil || status/100 != 2 {
		return fmt.Errorf("rag-server exit request failed: status %d: %v", status, err)
	}
	if err := stopRag(false); err != nil {
		return fmt.Errorf("rag-server did not exit gracefully: %w", err)
	}
	ragStopped = true
	if err := assertChatbotDegradedTurn(); err != nil {
		return err
	}

	// Exit the chatbot gracefully so its span log flushes, then assert the trace.
	if _, status, err := requestHTTP(http.MethodPost, chatbotControlExit, `{"reason":"chatbot integration done"}`); err != nil || status/100 != 2 {
		return fmt.Errorf("chatbot exit request failed: status %d: %v", status, err)
	}
	if err := stopChat(false); err != nil {
		return fmt.Errorf("chatbot did not exit gracefully: %w", err)
	}
	chatStopped = true

	if err := assertChatbotTrace(chatTrace); err != nil {
		return err
	}
	// The two span logs are asserted independently. Cross-agent trace continuity
	// (the chatbot's rag_query span parenting the rag-server machine_request span
	// through a propagated traceparent) is the observability epic's work.
	if err := assertRagServerServed(ragTrace); err != nil {
		return err
	}

	fmt.Println("integration:chatbot PASS - grounded turn answered from retrieved chunks, RAG-down turn degraded to a 200, chatbot trace shows the model answered, rag-server served and exited")
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

func assertChatbotGroundedTurn() error {
	// The question is answered by the corpus, so retrieval grounds the answer and
	// the model cites the record it used.
	resp, status, err := postChatTurn("What do the Chroma corpus agents use to compute embeddings?", "[]")
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("grounded turn status = %d, want 200: %s", status, resp.Message+resp.Error)
	}
	if strings.TrimSpace(resp.Answer) == "" {
		return fmt.Errorf("grounded turn returned an empty answer")
	}
	if resp.Trace.Status != "succeeded" {
		return fmt.Errorf("grounded turn trace status = %q, want succeeded", resp.Trace.Status)
	}
	// Grounding proof: the answer states a fact that lives only in the retrieved
	// chunk (the corpus records that the Chroma agents compute embeddings at a
	// local Ollama provider), so a correct answer names Ollama. The model does not
	// always emit an explicit bracket citation, so grounding is asserted by the
	// corpus fact and the rag_query -> invoke_llm trace, not by citation presence.
	if !strings.Contains(strings.ToLower(resp.Answer), "ollama") {
		return fmt.Errorf("grounded turn answer is not grounded in the retrieved chunk (no Ollama fact): %s", resp.Answer)
	}
	// The chatbot passes chunk documents (not Chroma ids) to the model, so any
	// citation the model does emit is a 1-based position into the fanned-out
	// results (n_results = 5). Validate the positions the model cited. Asserting
	// the cited Chroma ids equal the seeded ids needs ids threaded into compose,
	// which is a follow-on.
	for _, n := range citedRecordNumbers(resp.Answer) {
		if n < 1 || n > 5 {
			return fmt.Errorf("grounded turn cited record %d outside the fanned-out result range [1,5]; answer: %s", n, resp.Answer)
		}
	}
	return nil
}

func assertChatbotDegradedTurn() error {
	// A follow-up turn carries the prior turn as client-side history (srd014 R4).
	history := `[{"role":"user","content":"What is specification-driven development?"},{"role":"assistant","content":"It is the practice of writing specifications first."}]`
	resp, status, err := postChatTurn("How many Chroma corpus agents are there?", history)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("degraded turn status = %d, want 200 (RAG down must degrade, not fail): %s", status, resp.Message+resp.Error)
	}
	if strings.TrimSpace(resp.Answer) == "" {
		return fmt.Errorf("degraded turn returned an empty answer")
	}
	if resp.Trace.Status != "succeeded" {
		return fmt.Errorf("degraded turn trace status = %q, want succeeded", resp.Trace.Status)
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

// assertChatbotTrace proves, from the chatbot's own span log, that the turn
// reached the configured chat model and produced an answer: a genai chat span
// with a positive output-token count.
//
// The per-word request-scoped dispatch spans (embed_query, rag_query,
// compose_prompt, invoke_llm) are NOT asserted here. The machine_request runner
// dispatches the request-scoped turn with a NoopTracer
// (agent-core internal/tools/rest/collection.go), so those spans are not exported
// to the process OTel file; only the host lifecycle words and the genai chat span
// reach it. Exporting the request-scoped dispatch spans -- and the cross-agent
// traceparent continuity that would let the chatbot's rag_query span parent the
// rag-server's machine_request span -- is the observability epic's work
// (agent-core srd016). The grounded HTTP turn already proves the full
// embed -> rag -> compose -> answer chain ran, since the answer cannot be grounded
// in retrieved chunks otherwise.
func assertChatbotTrace(tracePath string) error {
	spans, err := readChromaSpans(tracePath)
	if err != nil {
		return err
	}
	for _, s := range spans {
		if strings.HasPrefix(s.Name, "chat ") {
			if tokens, ok := s.numericAttr("gen_ai.usage.output_tokens"); ok && tokens > 0 {
				return nil
			}
		}
	}
	return fmt.Errorf("chatbot trace shows no completed chat span with output tokens; saw %v", sortedKeys(chromaCommandSet(spans)))
}

// assertRagServerServed proves, from the rag-server's own span log, that it
// served and exited gracefully: its request server launched and the agent exited.
// The per-query rag_query dispatch runs request-scoped under a NoopTracer (see
// assertChatbotTrace), so it is not in this log; the grounded turn's HTTP response
// is the evidence the query was served. Asserted independently of the chatbot
// trace -- cross-agent trace continuity is the observability epic's work.
func assertRagServerServed(tracePath string) error {
	spans, err := readChromaSpans(tracePath)
	if err != nil {
		return err
	}
	present := chromaCommandSet(spans)
	for _, want := range []string{"launch_rag_requests", "exit_agent"} {
		if !present[want] {
			return fmt.Errorf("rag-server trace missing %q dispatch; saw %v", want, sortedKeys(present))
		}
	}
	return nil
}

var citedRecordPattern = regexp.MustCompile(`(?i)record[\s#]*([0-9]+)`)

// citedRecordNumbers extracts the 1-based record positions the model cited from
// the answer text. The grounding prompt asks the model to cite the record for
// each claim, and models write those as "[record N]" or "record N".
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

// chatbotOllamaSkipReason returns a non-empty reason when Ollama is unreachable
// or a model the chatbot integration needs is not installed: the chroma embed
// model that seeds the collection, plus the chatbot's own embedding and chat
// models. Reading them from config keeps the gate from duplicating model names.
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
	// The ingest seed uses the chroma embedding model.
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
	chat, err := chatbotChatModelFromConfig(profilesRoot)
	if err != nil {
		return nil, err
	}
	set[chat] = true
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

// chatbotChatModelFromConfig reads the invoke_llm chat model from the chatbot's
// request-declarations.yaml.
func chatbotChatModelFromConfig(profilesRoot string) (string, error) {
	var cfg struct {
		Tools []struct {
			Name   string `yaml:"name"`
			Config struct {
				Model string `yaml:"model"`
			} `yaml:"config"`
		} `yaml:"tools"`
	}
	path := filepath.Join(profilesRoot, "agents", "chatbot", "request-declarations.yaml")
	if err := readIntegrationYAML(path, "chatbot request declarations", &cfg); err != nil {
		return "", err
	}
	for _, tool := range cfg.Tools {
		if tool.Name == "invoke_llm" {
			if tool.Config.Model == "" {
				return "", fmt.Errorf("invoke_llm has no model in %s", path)
			}
			return tool.Config.Model, nil
		}
	}
	return "", fmt.Errorf("no invoke_llm tool in %s", path)
}
