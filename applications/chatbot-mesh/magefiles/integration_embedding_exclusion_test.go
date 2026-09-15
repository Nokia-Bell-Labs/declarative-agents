// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateShiftedChatbotProfileStagesImportedTypeUnits(t *testing.T) {
	applicationRoot := filepath.Clean("..")
	work := t.TempDir()

	profile, err := generateShiftedChatbotProfile(applicationRoot, work)
	if err != nil {
		t.Fatal(err)
	}
	if profile != filepath.Join(work, "chatbot-shifted", "profile.yaml") {
		t.Fatalf("profile = %s, want shifted profile under work root", profile)
	}

	source := filepath.Join(applicationRoot, "agents", "units", "types-chatbot.yaml")
	staged := filepath.Join(work, "units", "types-chatbot.yaml")
	want, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(staged)
	if err != nil {
		t.Fatalf("read staged type unit: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Error("staged types-chatbot.yaml differs from its source")
	}
}

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
