// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"encoding/json"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSeedRagServerCorpusUsesDeterministicDirectOperations(t *testing.T) {
	root := t.TempDir()
	corpusDir := filepath.Join(root, chromaCorpusFixture, "corpus")
	if err := os.MkdirAll(corpusDir, 0o755); err != nil {
		t.Fatal(err)
	}
	wantIDs := []string{"spec-driven-development.md", "chroma-corpus-agents.md"}
	wantPaths := []string{"spec-driven-development.md", "chroma-corpus-agents.md"}
	if len(ragServerSeedFiles) != len(wantPaths) {
		t.Fatalf("seed manifest has %d files, want %d", len(ragServerSeedFiles), len(wantPaths))
	}
	wantDocs := []string{"spec fixture\n", "chroma fixture\n"}
	for i, fixture := range ragServerSeedFiles {
		if fixture.id != wantIDs[i] || fixture.path != wantPaths[i] {
			t.Fatalf("seed file %d = (%q, %q), want (%q, %q)",
				i, fixture.id, fixture.path, wantIDs[i], wantPaths[i])
		}
		if err := os.WriteFile(filepath.Join(corpusDir, fixture.path), []byte(wantDocs[i]), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(corpusDir, "unselected.md"), []byte("must not be embedded"), 0o644); err != nil {
		t.Fatal(err)
	}

	var embedded []string
	var addPayload struct {
		IDs        []string    `json:"ids"`
		Documents  []string    `json:"documents"`
		Embeddings [][]float64 `json:"embeddings"`
	}
	collectionRequests := 0
	err := seedRagServerCorpusWithOperations(root, "embed-model", ragServerSeedOperations{
		embed: func(model, text string) ([]float64, error) {
			if model != "embed-model" {
				t.Fatalf("embedding model = %q, want embed-model", model)
			}
			embedded = append(embedded, text)
			return []float64{float64(len(embedded)), 2, 3}, nil
		},
		request: func(method, url, body string) ([]byte, int, error) {
			if method != http.MethodPost {
				t.Fatalf("request method = %q, want POST", method)
			}
			collectionRequests++
			if collectionRequests == 1 {
				if !strings.HasSuffix(url, "/collections") {
					t.Fatalf("collection URL = %q", url)
				}
				if body != `{"name":"corpus","get_or_create":true}` {
					t.Fatalf("collection body = %s", body)
				}
				return []byte(`{"id":"fixture-collection"}`), http.StatusCreated, nil
			}
			if !strings.HasSuffix(url, "/collections/fixture-collection/add") {
				t.Fatalf("add URL = %q", url)
			}
			if err := json.Unmarshal([]byte(body), &addPayload); err != nil {
				t.Fatalf("decode add payload: %v", err)
			}
			return nil, http.StatusCreated, nil
		},
	})
	if err != nil {
		t.Fatalf("seedRagServerCorpusWithOperations: %v", err)
	}
	if collectionRequests != 2 {
		t.Fatalf("Chroma requests = %d, want create/get then add", collectionRequests)
	}
	if !slices.Equal(embedded, wantDocs) || !slices.Equal(addPayload.Documents, wantDocs) {
		t.Fatalf("seeded documents = %q / %q, want fixed manifest %q", embedded, addPayload.Documents, wantDocs)
	}
	if !slices.Equal(addPayload.IDs, wantIDs) {
		t.Fatalf("seed ids = %v, want %v", addPayload.IDs, wantIDs)
	}
	for i, vector := range addPayload.Embeddings {
		if len(vector) != 3 {
			t.Fatalf("embedding %d dimension = %d, want 3", i, len(vector))
		}
	}
}

func TestRagServerSetupHasNoModelDrivenIngestDependency(t *testing.T) {
	source, err := os.ReadFile("integration_ragserver.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"runChromaIngest(", "chromaRequiredModels("} {
		if strings.Contains(string(source), forbidden) {
			t.Errorf("rag-server setup contains model-driven ingest dependency %q", forbidden)
		}
	}
}

func TestParseRagQueryResponseChunksAndMetadata(t *testing.T) {
	body := []byte(`{
		"ids": [["doc-1","doc-2","doc-3"]],
		"documents": [["about apples","about bananas","about cherries"]],
		"distances": [[0.02,1.62,1.82]],
		"embedding_model": "qwen3-embedding:8b",
		"trace": {"iterations": 2, "terminal_signal": "QueryResponded", "status": "succeeded"}
	}`)
	resp, err := parseRagQueryResponse(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := resp.chunkCount(); got != 3 {
		t.Fatalf("chunkCount = %d, want 3", got)
	}
	if resp.EmbeddingModel != "qwen3-embedding:8b" {
		t.Fatalf("embedding_model = %q, want qwen3-embedding:8b", resp.EmbeddingModel)
	}
	if resp.Trace.TerminalSignal != "QueryResponded" {
		t.Fatalf("terminal_signal = %q, want QueryResponded", resp.Trace.TerminalSignal)
	}
	if resp.Trace.Iterations != 2 {
		t.Fatalf("iterations = %d, want 2", resp.Trace.Iterations)
	}
	if err := resp.validateAlignment(); err != nil {
		t.Fatalf("validateAlignment: %v", err)
	}
}

func TestValidateAlignment(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name: "aligned ids documents and distances pass",
			body: `{"ids":[["a","b"]],"documents":[["doc a","doc b"]],"distances":[[0.1,0.9]]}`,
		},
		{
			name:    "no ids array is rejected",
			body:    `{"ids":[],"documents":[],"distances":[]}`,
			wantErr: "no ids array",
		},
		{
			name:    "missing documents outer dimension is rejected",
			body:    `{"ids":[["a","b"]],"distances":[[0.1,0.9]]}`,
			wantErr: "documents outer dimension",
		},
		{
			name:    "missing distances outer dimension is rejected",
			body:    `{"ids":[["a","b"]],"documents":[["doc a","doc b"]]}`,
			wantErr: "distances outer dimension",
		},
		{
			name:    "documents inner dimension mismatch is rejected",
			body:    `{"ids":[["a","b"]],"documents":[["only one"]],"distances":[[0.1,0.9]]}`,
			wantErr: "documents inner dimension",
		},
		{
			name:    "distances inner dimension mismatch is rejected",
			body:    `{"ids":[["a","b"]],"documents":[["doc a","doc b"]],"distances":[[0.1]]}`,
			wantErr: "distances inner dimension",
		},
		{
			name:    "empty document alongside an id is rejected",
			body:    `{"ids":[["a","b"]],"documents":[["doc a","   "]],"distances":[[0.1,0.9]]}`,
			wantErr: "empty document",
		},
		{
			name: "non-finite distance is rejected",
			// JSON has no Inf literal; a distance array shorter cannot express it,
			// so drive it through a constructed response below instead.
			body:    "",
			wantErr: "non-finite distance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp ragQueryResponse
			if tt.body == "" {
				// Construct a non-finite distance directly since JSON lacks Inf.
				resp = ragQueryResponse{
					IDs:       [][]string{{"a"}},
					Documents: [][]string{{"doc a"}},
					Distances: [][]float64{{math.Inf(1)}},
				}
			} else {
				var err error
				resp, err = parseRagQueryResponse([]byte(tt.body))
				if err != nil {
					t.Fatalf("parse: %v", err)
				}
			}
			err := resp.validateAlignment()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateAlignment: unexpected error %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateAlignment error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestParseRagQueryResponseEmptyChunks(t *testing.T) {
	resp, err := parseRagQueryResponse([]byte(`{"ids": [[]], "embedding_model": "m", "trace": {"terminal_signal": "QueryResponded"}}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := resp.chunkCount(); got != 0 {
		t.Fatalf("chunkCount = %d, want 0", got)
	}
}

func TestParseRagQueryResponseNoIDs(t *testing.T) {
	resp, err := parseRagQueryResponse([]byte(`{"embedding_model": "m"}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := resp.chunkCount(); got != 0 {
		t.Fatalf("chunkCount = %d, want 0", got)
	}
}

func TestRagQueryBodyMarshalsVector(t *testing.T) {
	body, err := ragQueryBody([]float64{0.1, 0.2, 0.3}, 5)
	if err != nil {
		t.Fatalf("body: %v", err)
	}
	var payload struct {
		NResults        int       `json:"n_results"`
		QueryEmbeddings []float64 `json:"query_embeddings"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if payload.NResults != 5 {
		t.Fatalf("n_results = %d, want 5", payload.NResults)
	}
	if want := []float64{0.1, 0.2, 0.3}; !slices.Equal(payload.QueryEmbeddings, want) {
		t.Fatalf("query_embeddings = %v, want %v", payload.QueryEmbeddings, want)
	}
}
