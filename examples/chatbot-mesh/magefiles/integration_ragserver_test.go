// Copyright (c) 2026 Nokia. All rights reserved.

package main

import "testing"

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
	want := `{"n_results":5,"query_embeddings":[0.1,0.2,0.3]}`
	if body != want {
		t.Fatalf("ragQueryBody = %s, want %s", body, want)
	}
}
