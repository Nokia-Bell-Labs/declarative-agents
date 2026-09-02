// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"encoding/json"
	"testing"
)

func TestAssertExclusionMetadataReadsProjectedSources(t *testing.T) {
	raw := `{
		"answer": "grounded",
		"metadata": {
			"query_embedding_model": "qwen3-embedding:8b",
			"sources": {
				"not_selected": [],
				"composed": [{"name":"rag1","signal":"QueryResponded","documents":[["kept"]],"ids":[["rag1-doc-1"]]}],
				"embedding_model_excluded": [{"name":"rag0","embedding_model":"nomic-embed-text:v1.5"}],
				"query_failed": []
			}
		}
	}`
	var response exclusionResponse
	if err := json.Unmarshal([]byte(raw), &response); err != nil {
		t.Fatalf("decode projected response: %v", err)
	}
	if err := assertExclusionMetadata(response); err != nil {
		t.Fatalf("projected exclusion metadata rejected: %v", err)
	}
}
