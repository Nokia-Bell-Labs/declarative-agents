// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	ragServerProfile = "agents/rag-server/profile.yaml"

	ragServerCollection = "corpus"
	ragQueryURL         = "http://127.0.0.1:18085/api/v1/rag/query"
	ragControlHealth    = "http://127.0.0.1:18086/api/lifecycle/health"
	ragControlExit      = "http://127.0.0.1:18086/api/lifecycle/exit"
	ragMonitorState     = "http://127.0.0.1:18087/monitor/state"
	ollamaEmbedURL      = "http://127.0.0.1:11434/api/embeddings"
)

var ragServerSeedFiles = []struct {
	id   string
	path string
}{
	{"spec-driven-development.md", "spec-driven-development.md"},
	{"chroma-corpus-agents.md", "chroma-corpus-agents.md"},
}

// RagServer proves the persistent RAG service end to end. It starts a Chroma
// container, deterministically embeds and writes the two serving fixtures,
// launches the rag-server as a long-running subprocess, then acts as the caller:
// it embeds a query at Ollama to obtain a matching-dimension vector, posts it to
// the machine_request query endpoint, and asserts the returned chunks and
// embedding-model metadata, a mapped rejection for a wrong-dimension vector,
// and a reachable monitor view. It requests a graceful lifecycle exit and
// asserts the process stops. The target skips (does not fail) when Docker or
// Ollama with the configured embedding model is unavailable.
func (Integration) RagServer() error {
	profilesRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := demoCoreRoot(profilesRoot)
	if err := requireProfilePaths(profilesRoot, ragServerProfile, corpusRestAsset); err != nil {
		return err
	}
	embedModel, err := chromaEmbedModelFromConfig(profilesRoot)
	if err != nil {
		return fmt.Errorf("invalid shipped RAG model config: %w", err)
	}
	if reason := chromaOllamaSkipReasonForModels([]string{embedModel}); reason != "" {
		fmt.Printf("SKIP ragServer: %s\n", reason)
		return nil
	}
	if _, err := exec.LookPath("docker"); err != nil {
		fmt.Println("SKIP ragServer: docker not found on PATH")
		return nil
	}
	return runRagServerIntegration(profilesRoot, coreRoot, embedModel)
}

func runRagServerIntegration(profilesRoot, coreRoot, embedModel string) error {
	binary, err := buildAgent(coreRoot)
	if err != nil {
		return err
	}
	dataDir, err := os.MkdirTemp("", "chatbot-mesh-ragserver-data-*")
	if err != nil {
		return fmt.Errorf("create chroma data dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(dataDir) }()
	containerID, err := startRequiredChromaContainer(dataDir, ensureChromaServer)
	if err != nil {
		return fmt.Errorf("rag-server dependency startup: %w", err)
	}
	defer stopChromaContainer(containerID)

	// Seed exactly the shipped serving fixture. Integration.Chroma independently
	// proves the canonical model-driven corpus-ingest workflow.
	if err := seedRagServerCorpus(profilesRoot, embedModel); err != nil {
		return fmt.Errorf("seed rag-server corpus: %w", err)
	}

	// Embed a query at Ollama so the query vector matches the corpus dimension.
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
	if err := assertRagQueryHonorsNResults(vector); err != nil {
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

type ragServerSeedOperations struct {
	embed   func(model, text string) ([]float64, error)
	request func(method, url, body string) ([]byte, int, error)
}

func seedRagServerCorpus(profilesRoot, embedModel string) error {
	return seedRagServerCorpusWithOperations(profilesRoot, embedModel, ragServerSeedOperations{
		embed:   ollamaEmbedQuery,
		request: requestHTTP,
	})
}

// seedRagServerCorpusWithOperations reads a fixed manifest rather than asking a
// model to select files. It embeds the exact canonical fixture documents with
// the configured model and writes those vectors directly to the served Chroma
// collection. Embedding every document and the query with one model preserves
// the collection's vector-dimension contract.
func seedRagServerCorpusWithOperations(profilesRoot, embedModel string, ops ragServerSeedOperations) error {
	base := "http://127.0.0.1:8000/api/v2/tenants/default_tenant/databases/default_database/collections"
	data, status, err := ops.request(http.MethodPost, base,
		fmt.Sprintf(`{"name":%q,"get_or_create":true}`, ragServerCollection))
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return fmt.Errorf("resolve %s collection: status %d: %s", ragServerCollection, status, data)
	}
	var collection struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &collection); err != nil {
		return fmt.Errorf("decode collection id: %w", err)
	}

	ids := make([]string, 0, len(ragServerSeedFiles))
	documents := make([]string, 0, len(ragServerSeedFiles))
	embeddings := make([][]float64, 0, len(ragServerSeedFiles))
	dimension := 0
	for _, fixture := range ragServerSeedFiles {
		content, err := os.ReadFile(filepath.Join(
			profilesRoot, chromaCorpusFixture, "corpus", fixture.path))
		if err != nil {
			return fmt.Errorf("read rag-server fixture %s: %w", fixture.path, err)
		}
		vector, err := ops.embed(embedModel, string(content))
		if err != nil {
			return fmt.Errorf("embed rag-server fixture %s: %w", fixture.path, err)
		}
		if dimension == 0 {
			dimension = len(vector)
		} else if len(vector) != dimension {
			return fmt.Errorf("fixture %s embedding dimension = %d, want %d",
				fixture.path, len(vector), dimension)
		}
		ids = append(ids, fixture.id)
		documents = append(documents, string(content))
		embeddings = append(embeddings, vector)
	}
	payload, err := json.Marshal(map[string]interface{}{
		"ids": ids, "documents": documents, "embeddings": embeddings,
	})
	if err != nil {
		return err
	}
	addData, addStatus, err := ops.request(
		http.MethodPost, base+"/"+collection.ID+"/add", string(payload))
	if err != nil {
		return err
	}
	if addStatus/100 != 2 {
		return fmt.Errorf("add to %s: status %d: %s", ragServerCollection, addStatus, addData)
	}
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
	args := []string{
		"--profile", filepath.Join(profilesRoot, ragServerProfile),
		"--directory", os.TempDir(),
		"--core-root", coreRoot,
		"--otel-log-file", trace,
	}
	telemetryArgs, resourceEnv := hostIntegrationTelemetry("integration:ragServer", "rag-server", profilesRoot)
	cmd := exec.Command(binary, append(args, telemetryArgs...)...)
	cmd.Dir = profilesRoot
	cmd.Env = append(os.Environ(), resourceEnv)
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
	data, status, err := requestInference(http.MethodPost, ollamaEmbedURL, body, "embed query vector with model "+model)
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
	if resp.chunkCount() > 3 {
		return fmt.Errorf("rag query returned %d chunks for n_results=3; request count not honored: %s", resp.chunkCount(), data)
	}
	if err := resp.validateAlignment(); err != nil {
		return fmt.Errorf("rag query result unusable: %w: %s", err, data)
	}
	if resp.EmbeddingModel == "" {
		return fmt.Errorf("rag query response is missing the embedding_model metadata: %s", data)
	}
	if resp.Trace.TerminalSignal != "QueryResponded" {
		return fmt.Errorf("rag query terminal signal = %q, want QueryResponded", resp.Trace.TerminalSignal)
	}
	return nil
}

// assertRagQueryHonorsNResults proves the request-supplied n_results caps the
// returned chunk count. n_results=1 must yield at most one chunk, which the old
// hard-coded n_results=5 could not satisfy against a multi-document corpus
// (srd001 AC2; GH-501).
func assertRagQueryHonorsNResults(vector []float64) error {
	body, err := ragQueryBody(vector, 1)
	if err != nil {
		return err
	}
	data, status, err := requestHTTP(http.MethodPost, ragQueryURL, body)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("rag query n_results=1 returned status %d: %s", status, data)
	}
	resp, err := parseRagQueryResponse(data)
	if err != nil {
		return err
	}
	if resp.chunkCount() != 1 {
		return fmt.Errorf("rag query n_results=1 returned %d chunks, want exactly 1: %s", resp.chunkCount(), data)
	}
	if err := resp.validateAlignment(); err != nil {
		return fmt.Errorf("rag query n_results=1 result unusable: %w: %s", err, data)
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

// validateAlignment enforces that the query result is actually usable rather
// than merely carrying IDs. Chroma returns ids, documents, and distances as
// parallel arrays, so a real chunk must have a nonempty document and a finite
// distance at the same position. It requires matching outer and inner
// dimensions across all three arrays, a nonempty document per chunk, and a
// finite distance per chunk (srd rel00.0: chunks carry IDs and distances).
func (r ragQueryResponse) validateAlignment() error {
	if len(r.IDs) == 0 {
		return fmt.Errorf("rag query result carries no ids array")
	}
	if len(r.Documents) != len(r.IDs) {
		return fmt.Errorf("documents outer dimension %d != ids outer dimension %d",
			len(r.Documents), len(r.IDs))
	}
	if len(r.Distances) != len(r.IDs) {
		return fmt.Errorf("distances outer dimension %d != ids outer dimension %d",
			len(r.Distances), len(r.IDs))
	}
	for row := range r.IDs {
		ids, docs, dists := r.IDs[row], r.Documents[row], r.Distances[row]
		if len(docs) != len(ids) {
			return fmt.Errorf("row %d: documents inner dimension %d != ids inner dimension %d",
				row, len(docs), len(ids))
		}
		if len(dists) != len(ids) {
			return fmt.Errorf("row %d: distances inner dimension %d != ids inner dimension %d",
				row, len(dists), len(ids))
		}
		for chunk, doc := range docs {
			if strings.TrimSpace(doc) == "" {
				return fmt.Errorf("row %d chunk %d: empty document alongside id %q",
					row, chunk, ids[chunk])
			}
		}
		for chunk, dist := range dists {
			if math.IsNaN(dist) || math.IsInf(dist, 0) {
				return fmt.Errorf("row %d chunk %d: non-finite distance %v alongside id %q",
					row, chunk, dist, ids[chunk])
			}
		}
	}
	return nil
}

func parseRagQueryResponse(data []byte) (ragQueryResponse, error) {
	var resp ragQueryResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return ragQueryResponse{}, fmt.Errorf("decode rag query response: %w", err)
	}
	return resp, nil
}
