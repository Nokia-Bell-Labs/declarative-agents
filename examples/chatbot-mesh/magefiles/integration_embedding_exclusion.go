// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The two RAG mocks answer with the same vector shape and differ only in the
// embedding model they report. rag0's identity does not match the query's, so
// srd002 R3.3 excludes it; rag1's matches, so it must survive in the same turn.
const (
	exclusionQueryModel    = "qwen3-embedding:8b"
	exclusionForeignModel  = "nomic-embed-text:v1.5"
	exclusionExcludedChunk = "rag0 chunk from a foreign embedding space"
	exclusionKeptChunk     = "rag1 chunk from the query embedding space"
)

// The shipped ports, and the offset that moves the whole set aside so the proof
// does not contend with a real Ollama or a running mesh. Every one of these
// appears in both a client base_url and a network limit, so one replacement
// keeps the declared authority self-consistent.
const (
	portEmbedding      = "11434"
	portRag0           = "18085"
	portRag1           = "18095"
	portChat           = "18080"
	portControl        = "18081"
	portMonitor        = "18082"
	exclusionPortShift = 20000
)

// exclusionPort shifts a shipped port for this proof's isolated run.
func exclusionPort(shipped string) string {
	n, err := strconv.Atoi(shipped)
	if err != nil {
		return shipped
	}
	return strconv.Itoa(n + exclusionPortShift)
}

func exclusionAddr(shipped string) string { return "127.0.0.1:" + exclusionPort(shipped) }

func exclusionURL(shipped string) string { return "http://" + exclusionAddr(shipped) }

// generateShiftedChatbotProfile copies the chatbot profile with every shipped
// port shifted, so the client base_urls and the network limits that pin them
// move together. Relative references inside the copied files resolve against
// the profile's own directory; the launch still runs from the example root, so
// repository-relative paths such as the UX bundle keep resolving.
func generateShiftedChatbotProfile(exampleRoot, work string) (string, error) {
	srcDir := filepath.Join(exampleRoot, "agents", "chatbot")
	dstDir := filepath.Join(work, "chatbot-shifted")
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return "", err
	}
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return "", fmt.Errorf("read chatbot profile: %w", err)
	}
	pairs := []string{}
	for _, p := range []string{portEmbedding, portRag0, portRag1, portChat, portControl, portMonitor} {
		pairs = append(pairs, p, exclusionPort(p))
	}
	replacer := strings.NewReplacer(pairs...)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		content, err := os.ReadFile(filepath.Join(srcDir, entry.Name()))
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(dstDir, entry.Name()), []byte(replacer.Replace(string(content))), 0o644); err != nil {
			return "", err
		}
	}
	return filepath.Join(dstDir, "profile.yaml"), nil
}

// EmbeddingExclusion proves the exclusion the mapped-400 path cannot catch: two
// RAG sources answer 200 with the same vector shape, and only their reported
// embedding-model identities differ (srd002 R3.3, GH-767).
//
// The proof reads the composed prompt out of the LLM mock's request log rather
// than the chat answer. The answer is canned fixture text, so it is the same
// whichever chunks were composed; the prompt is the only place the exclusion is
// observable. Asserting the excluded chunk is absent AND the kept chunk is
// present is what distinguishes a real exclusion from a turn that simply lost
// both sources.
func (Integration) EmbeddingExclusion() error {
	exampleRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := envOrDefault("AGENT_CORE_ROOT", siblingPath(exampleRoot, "agent-core"))
	if !agentCoreAvailable(coreRoot) {
		fmt.Printf("SKIP integration:embeddingExclusion: agent-core checkout not found at %s\n", coreRoot)
		return nil
	}
	mockProfile := filepath.Join(siblingPath(exampleRoot, "agent-profiles"), "agents", "mock", "profile.yaml")
	if _, statErr := os.Stat(mockProfile); statErr != nil {
		fmt.Printf("SKIP integration:embeddingExclusion: mock profile not found at %s\n", mockProfile)
		return nil
	}
	binary, err := buildAgent(coreRoot)
	if err != nil {
		return err
	}

	work, err := os.MkdirTemp("", "embedding-exclusion")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(work) }()

	// The chatbot's declared ports are a transport-authority boundary, pinned in
	// both the client base_urls and the network limits (srd002 R6.1), so the
	// mocks cannot simply be moved aside. A port-shifted profile copy keeps that
	// boundary intact while freeing the proof from whatever already holds the
	// shipped ports -- a developer's real Ollama on 11434, most often. Same
	// device as the rag1 variant this example already generates.
	chatbotProfile, err := generateShiftedChatbotProfile(exampleRoot, work)
	if err != nil {
		return err
	}

	stops, err := startExclusionMocks(binary, coreRoot, mockProfile, work)
	defer stopAll(stops)
	if err != nil {
		return err
	}

	stopChatbot, err := startDetachedAgentWithEnv(agentLaunch{
		Binary: binary, ProfilesRoot: exampleRoot, CoreRoot: coreRoot,
		Profile:      chatbotProfile,
		TracePath:    filepath.Join(work, "chatbot.otel.json"),
		Workdir:      work,
		Env:          []string{"CHATBOT_EMBEDDING_MODEL=" + exclusionQueryModel},
		GracefulWait: 15 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("start chatbot: %w", err)
	}
	stops = append(stops, stopChatbot)

	if err := waitHTTPStatus(exclusionURL(portControl)+"/api/lifecycle/health", 200, 30*time.Second); err != nil {
		return fmt.Errorf("chatbot never became healthy: %w", err)
	}

	// The turn must still answer: an excluded source degrades the grounding, it
	// does not fail the turn (srd002 R3.2).
	resp, status, err := postExclusionChatTurn("what does the corpus say about the rig?")
	if err != nil {
		return fmt.Errorf("chat turn: %w", err)
	}
	if status != 200 {
		return fmt.Errorf("chat turn status = %d, want 200: an excluded source must degrade rather than fail the turn", status)
	}
	if strings.TrimSpace(resp.Answer) == "" {
		return fmt.Errorf("chat turn returned an empty answer")
	}

	prompt, err := composedPromptFromMockLog(exclusionURL(portEmbedding) + "/_mock/log")
	if err != nil {
		return err
	}
	if strings.Contains(prompt, exclusionExcludedChunk) {
		return fmt.Errorf("the composed prompt carries the chunk from the mismatched source (%q reported %q against a %q query); srd002 R3.3 excludes it:\n%s",
			exclusionExcludedChunk, exclusionForeignModel, exclusionQueryModel, prompt)
	}
	if !strings.Contains(prompt, exclusionKeptChunk) {
		return fmt.Errorf("the composed prompt lost the chunk from the matching source; the exclusion must drop only the mismatched one:\n%s", prompt)
	}

	fmt.Printf("integration:embeddingExclusion passed: %q excluded (reported %s), %q composed (reported %s)\n",
		exclusionExcludedChunk, exclusionForeignModel, exclusionKeptChunk, exclusionQueryModel)
	return nil
}

// startExclusionMocks stands the three dependencies the chatbot's declared
// clients reach at their fixed addresses: the embedding and chat provider, and
// the two RAG servers.
func startExclusionMocks(binary, coreRoot, mockProfile, work string) ([]func(bool) error, error) {
	var stops []func(bool) error
	for _, m := range []struct {
		name    string
		address string
		fixture string
	}{
		{name: "llm", address: exclusionAddr(portEmbedding), fixture: exclusionLLMFixture},
		{name: "rag0", address: exclusionAddr(portRag0), fixture: fmt.Sprintf(exclusionRagFixture, "rag0-doc-1", exclusionExcludedChunk, exclusionForeignModel)},
		{name: "rag1", address: exclusionAddr(portRag1), fixture: fmt.Sprintf(exclusionRagFixture, "rag1-doc-1", exclusionKeptChunk, exclusionQueryModel)},
	} {
		path := filepath.Join(work, m.name+".yaml")
		if err := os.WriteFile(path, []byte(m.fixture), 0o644); err != nil {
			return stops, err
		}
		stop, err := startDetachedAgentWithEnv(agentLaunch{
			Binary: binary, ProfilesRoot: filepath.Dir(mockProfile), CoreRoot: coreRoot,
			Profile:      mockProfile,
			TracePath:    filepath.Join(work, m.name+".otel.json"),
			Workdir:      work,
			Env:          []string{"MOCK_ADDRESS=" + m.address, "MOCK_FIXTURES=" + path},
			GracefulWait: 10 * time.Second,
		})
		if err != nil {
			return stops, fmt.Errorf("start %s mock: %w", m.name, err)
		}
		stops = append(stops, stop)
		if err := waitHTTPStatus("http://"+m.address+"/_mock/health", 200, 30*time.Second); err != nil {
			return stops, fmt.Errorf("%s mock never became healthy: %w", m.name, err)
		}
	}
	return stops, nil
}

// postExclusionChatTurn posts one turn to the port-shifted chatbot. It does not
// reuse postChatTurn, which addresses the shipped chat port.
func postExclusionChatTurn(message string) (chatResponse, int, error) {
	body, err := json.Marshal(map[string]string{"message": message})
	if err != nil {
		return chatResponse{}, 0, err
	}
	data, status, err := requestInference("POST", exclusionURL(portChat)+"/api/v1/chat", string(body), "chat turn")
	if err != nil {
		return chatResponse{}, status, err
	}
	var resp chatResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return chatResponse{}, status, fmt.Errorf("decode chat response: %w: %s", err, data)
	}
	return resp, status, nil
}

func stopAll(stops []func(bool) error) {
	for i := len(stops) - 1; i >= 0; i-- {
		_ = stops[i](true)
	}
}

// composedPromptFromMockLog returns the body of the last POST /api/chat the
// subject sent. The router call and the answer call both land on that route;
// the last one carries the grounding prompt the answer was composed from.
func composedPromptFromMockLog(logURL string) (string, error) {
	data, status, err := requestHTTP("GET", logURL, "")
	if err != nil {
		return "", fmt.Errorf("read mock log: %w", err)
	}
	if status != 200 {
		return "", fmt.Errorf("mock log status = %d, want 200: %s", status, data)
	}
	var log struct {
		Requests []struct {
			Method string `json:"method"`
			Path   string `json:"path"`
			Body   string `json:"body"`
		} `json:"requests"`
	}
	if err := json.Unmarshal(data, &log); err != nil {
		return "", fmt.Errorf("decode mock log: %w", err)
	}
	prompt := ""
	for _, entry := range log.Requests {
		if entry.Method == "POST" && entry.Path == "/api/chat" {
			prompt = entry.Body
		}
	}
	if prompt == "" {
		return "", fmt.Errorf("the LLM mock recorded no POST /api/chat; the turn never reached the answer step")
	}
	return prompt, nil
}

const exclusionLLMFixture = `# Embedding and chat provider for the exclusion proof. The embedding response
# carries only the vector, which is why the query's model identity comes from
# declare_query_model rather than from this response.
routes:
  - method: GET
    path: /api/tags
    responses:
      - status: 200
        body:
          models:
            - name: "qwen2.5:3b"
            - name: "ornith:9b"

  - method: POST
    path: /api/embeddings
    responses:
      - status: 200
        body:
          embedding: [0.11, 0.22, 0.33, 0.44]

  - method: POST
    path: /api/chat
    responses:
      - status: 200
        body:
          message:
            role: assistant
            content: '{"tool":"invoke_llm_fast"}'
          eval_count: 4
          prompt_eval_count: 12
      - status: 200
        body:
          message:
            role: assistant
            content: "The corpus describes the assembler rig and its mocks."
          eval_count: 11
          prompt_eval_count: 42
`

// exclusionRagFixture answers a query with one chunk and a reported embedding
// model. Both instances return the same vector shape, so nothing but the
// reported identity distinguishes them: a 400 would prove nothing here.
const exclusionRagFixture = `routes:
  - method: POST
    path: /api/v1/rag/query
    responses:
      - status: 200
        body:
          ids: [["%s"]]
          documents: [["%s"]]
          distances: [[0.1]]
          embedding_model: "%s"
`
