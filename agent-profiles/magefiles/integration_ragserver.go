// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	ragServerProfile = "agents/chroma/rag-server/profile.yaml"

	ragQueryURL      = "http://127.0.0.1:18085/api/v1/rag/query"
	ragControlHealth = "http://127.0.0.1:18086/api/lifecycle/health"
	ragControlExit   = "http://127.0.0.1:18086/api/lifecycle/exit"
	ragMonitorState  = "http://127.0.0.1:18087/monitor/state"
	ollamaEmbedURL   = "http://127.0.0.1:11434/api/embeddings"
)

// RagServer proves the persistent RAG service end to end. It starts a Chroma
// container, seeds a collection by running the ingest profile, launches the
// rag-server as a long-running subprocess, then acts as the caller: it embeds a
// query at Ollama to obtain a matching-dimension vector, posts it to the
// machine_request query endpoint, and asserts the returned chunks and
// embedding-model metadata, a mapped rejection for a wrong-dimension vector, and
// a reachable monitor view. It requests a graceful lifecycle exit and asserts
// the process stops. The target skips (does not fail) when Docker or Ollama with
// the configured models is unavailable, matching Integration.Chroma.
func (Integration) RagServer() error {
	profilesRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := envOrDefault(agentCoreRootEnv, filepath.Join(filepath.Dir(profilesRoot), "agent-core"))
	if err := requireProfilePaths(profilesRoot, ragServerProfile, chromaIngestProfile, "agents/chroma/rest.yaml"); err != nil {
		return err
	}
	if reason := chromaOllamaSkipReason(profilesRoot); reason != "" {
		fmt.Printf("SKIP ragServer: %s\n", reason)
		return nil
	}
	if _, err := exec.LookPath("docker"); err != nil {
		fmt.Println("SKIP ragServer: docker not found on PATH")
		return nil
	}
	return runRagServerIntegration(profilesRoot, coreRoot)
}

func runRagServerIntegration(profilesRoot, coreRoot string) error {
	binary, err := buildIntegrationAgent(coreRoot)
	if err != nil {
		return err
	}
	dataDir, err := os.MkdirTemp("", "agent-profiles-ragserver-data-*")
	if err != nil {
		return fmt.Errorf("create chroma data dir: %w", err)
	}
	defer os.RemoveAll(dataDir)
	containerID, err := startChromaContainer(dataDir)
	if err != nil {
		fmt.Printf("SKIP ragServer: %s\n", err)
		return nil
	}
	defer stopChromaContainer(containerID)

	// Seed the served collection through the ingest profile.
	if err := runChromaIngest(binary, profilesRoot, coreRoot); err != nil {
		return err
	}

	// Embed a query at Ollama so the query vector matches the corpus dimension.
	embedModel, err := chromaEmbedModelFromConfig(profilesRoot)
	if err != nil {
		return err
	}
	vector, err := ollamaEmbedQuery(embedModel, "What does the corpus describe?")
	if err != nil {
		return fmt.Errorf("embed query vector: %w", err)
	}

	stop, err := startRagServer(binary, profilesRoot, coreRoot)
	if err != nil {
		return err
	}
	stopped := false
	defer func() {
		if !stopped {
			_ = stop(true)
		}
	}()
	if err := waitHTTPStatus(ragControlHealth, http.StatusOK, 30*time.Second); err != nil {
		return fmt.Errorf("rag-server control health never came up: %w", err)
	}

	if err := assertRagQueryReturnsChunks(vector); err != nil {
		return err
	}
	if err := assertRagWrongDimensionRejected(len(vector)); err != nil {
		return err
	}
	if err := assertRagMonitorReachable(); err != nil {
		return err
	}

	// Request a graceful lifecycle exit and assert the process stops.
	if _, status, err := requestHTTP(http.MethodPost, ragControlExit, `{"reason":"integration done"}`); err != nil || status/100 != 2 {
		return fmt.Errorf("rag-server exit request failed: status %d: %v", status, err)
	}
	if err := stop(false); err != nil {
		return fmt.Errorf("rag-server did not exit gracefully: %w", err)
	}
	stopped = true

	fmt.Println("integration:ragServer PASS - vector-in query returned chunks with embedding-model metadata, wrong-dimension rejected, monitor reachable, graceful exit")
	return nil
}

// startRagServer launches the rag-server agent detached and returns a stop
// function. stop(kill=false) waits for a graceful exit within a timeout; stop
// (kill=true) force-kills. The agent serves until it receives a lifecycle exit.
func startRagServer(binary, profilesRoot, coreRoot string) (func(kill bool) error, error) {
	trace, cleanup, err := chromaTraceFile("ragserver")
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(binary,
		"--profile", filepath.Join(profilesRoot, ragServerProfile),
		"--directory", os.TempDir(),
		"--core-root", coreRoot,
		"--otel-log-file", trace,
	)
	cmd.Dir = profilesRoot
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		cleanup()
		return nil, fmt.Errorf("start rag-server: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	return func(kill bool) error {
		defer cleanup()
		if kill {
			_ = cmd.Process.Kill()
			<-done
			return nil
		}
		select {
		case <-done:
			return nil
		case <-time.After(15 * time.Second):
			_ = cmd.Process.Kill()
			<-done
			return fmt.Errorf("rag-server did not stop within 15s after exit request")
		}
	}, nil
}

// ollamaEmbedQuery embeds text at Ollama and returns the vector, so the caller
// supplies a matching-dimension query vector, mirroring the mesh where the
// chatbot embeds once and fans out.
func ollamaEmbedQuery(model, text string) ([]float64, error) {
	body := fmt.Sprintf(`{"model":%q,"prompt":%q}`, model, text)
	data, status, err := requestHTTP(http.MethodPost, ollamaEmbedURL, body)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("ollama embeddings status %d: %s", status, data)
	}
	var result struct {
		Embedding []float64 `json:"embedding"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	if len(result.Embedding) == 0 {
		return nil, fmt.Errorf("ollama returned an empty embedding")
	}
	return result.Embedding, nil
}

func assertRagQueryReturnsChunks(vector []float64) error {
	body, err := ragQueryBody(vector, 3)
	if err != nil {
		return err
	}
	data, status, err := requestHTTP(http.MethodPost, ragQueryURL, body)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("rag query returned status %d: %s", status, data)
	}
	resp, err := parseRagQueryResponse(data)
	if err != nil {
		return err
	}
	if resp.chunkCount() == 0 {
		return fmt.Errorf("rag query returned no chunks: %s", data)
	}
	if resp.EmbeddingModel == "" {
		return fmt.Errorf("rag query response is missing the embedding_model metadata: %s", data)
	}
	if resp.Trace.TerminalSignal != "QueryResponded" {
		return fmt.Errorf("rag query terminal signal = %q, want QueryResponded", resp.Trace.TerminalSignal)
	}
	return nil
}

func assertRagWrongDimensionRejected(dim int) error {
	// A vector one element short of the collection dimension is rejected by Chroma.
	short := make([]float64, dim-1)
	body, err := ragQueryBody(short, 3)
	if err != nil {
		return err
	}
	data, status, err := requestHTTP(http.MethodPost, ragQueryURL, body)
	if err != nil {
		return err
	}
	if status != http.StatusBadRequest {
		return fmt.Errorf("wrong-dimension query status = %d, want 400: %s", status, data)
	}
	return nil
}

func assertRagMonitorReachable() error {
	data, status, err := requestHTTP(http.MethodGet, ragMonitorState, "")
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("monitor current_state status = %d, want 200: %s", status, data)
	}
	return nil
}

func ragQueryBody(vector []float64, nResults int) (string, error) {
	payload := map[string]interface{}{"query_embeddings": vector, "n_results": nResults}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ragQueryResponse is the shape the rag-server query endpoint returns on success.
type ragQueryResponse struct {
	IDs            [][]string  `json:"ids"`
	Documents      [][]string  `json:"documents"`
	Distances      [][]float64 `json:"distances"`
	EmbeddingModel string      `json:"embedding_model"`
	Trace          struct {
		Iterations     int    `json:"iterations"`
		TerminalSignal string `json:"terminal_signal"`
		Status         string `json:"status"`
	} `json:"trace"`
}

func (r ragQueryResponse) chunkCount() int {
	if len(r.IDs) == 0 {
		return 0
	}
	return len(r.IDs[0])
}

func parseRagQueryResponse(data []byte) (ragQueryResponse, error) {
	var resp ragQueryResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return ragQueryResponse{}, fmt.Errorf("decode rag query response: %w", err)
	}
	return resp, nil
}
