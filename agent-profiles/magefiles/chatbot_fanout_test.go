// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// chatbotMachine is the subset of the chatbot request-machine needed to assert the
// fan-out relocation (GH-365): degradation and embedding-mismatch exclusion are
// visible machine transitions, not a merge word.
type chatbotMachine struct {
	States      []struct{ Name string } `yaml:"states"`
	Signals     []struct{ Name string } `yaml:"signals"`
	Transitions []struct {
		State   string `yaml:"state"`
		Signal  string `yaml:"signal"`
		Next    string `yaml:"next"`
		Action  string `yaml:"action"`
		ForEach *struct {
			Items   string `yaml:"items"`
			As      string `yaml:"as"`
			Mode    string `yaml:"mode"`
			Failure string `yaml:"failure"`
			Join    struct {
				Label string `yaml:"label"`
			} `yaml:"join"`
		} `yaml:"for_each"`
	} `yaml:"transitions"`
}

func readRequiredChatbotAsset(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read required chatbot asset %s: %v", path, err)
	}
	return data
}

func chatbotAssetPath(name string) string {
	return filepath.Join("..", "..", "examples", "chatbot-mesh", "agents", "chatbot", name)
}

func loadChatbotMachine(t *testing.T) chatbotMachine {
	t.Helper()
	path := chatbotAssetPath("request-machine.yaml")
	data := readRequiredChatbotAsset(t, path)
	var m chatbotMachine
	if err := yaml.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse request-machine.yaml: %v", err)
	}
	return m
}

// TestChatbotFanOutHasNoMergeWord locks that rag_merge is gone from the chatbot
// turn. partition is a generic ordered filter, not a domain merge word: it
// preserves matched and unmatched inputs instead of combining RAG payloads.
func TestChatbotFanOutHasNoMergeWord(t *testing.T) {
	m := loadChatbotMachine(t)
	for _, s := range m.States {
		if s.Name == "Merging" {
			t.Error("Merging state still present; rag_merge should be gone (GH-365)")
		}
	}
	for _, s := range m.Signals {
		if s.Name == "Merged" {
			t.Error("Merged signal still present; rag_merge should be gone (GH-365)")
		}
	}
	for _, tr := range m.Transitions {
		if tr.Action == "rag_merge" {
			t.Errorf("rag_merge action still present at (%s,%s)", tr.State, tr.Signal)
		}
	}
}

// TestChatbotFanOutRoutesDegradedAndExcluded locks one sequential iterator over
// the declared topology. QueryRejected and CommandError are collected as failed
// item outcomes, while QueryResponded is successful; generic partitions retain
// vector rejection, degradation, and model mismatch as distinct sets.
func TestChatbotFanOutRoutesDegradedAndExcluded(t *testing.T) {
	m := loadChatbotMachine(t)
	var iterators int
	for _, tr := range m.Transitions {
		if tr.ForEach == nil {
			continue
		}
		iterators++
		if tr.Action != "rag_query" || tr.ForEach.Items != "$from(declare_rag_topology).items" ||
			tr.ForEach.As != "rag_unit" || tr.ForEach.Mode != "sequential" ||
			tr.ForEach.Failure != "collect_all" || tr.ForEach.Join.Label != "rag_queries" {
			t.Errorf("unexpected chatbot iterator: %+v", tr)
		}
	}
	if iterators != 1 {
		t.Fatalf("chatbot machine has %d for_each transitions, want exactly one", iterators)
	}
	for _, indexed := range []string{"Retrieving0", "Retrieving1", "rag_query0", "rag_query1", "compare_model0", "keep_chunks0"} {
		for _, state := range m.States {
			if state.Name == indexed {
				t.Errorf("indexed fan-out state remains: %s", indexed)
			}
		}
		for _, tr := range m.Transitions {
			if tr.Action == indexed {
				t.Errorf("indexed fan-out action remains: %s", indexed)
			}
		}
	}
}

// TestChatbotComposeReadsEachRagSource locks the fixed-selector collection
// pipeline: query outcomes are partitioned by signal, successful structured
// outputs by embedding model, and only the compatible set is rendered.
func TestChatbotComposeReadsEachRagSource(t *testing.T) {
	fanout := chatbotAssetPath("request-fanout.yaml")
	data := readRequiredChatbotAsset(t, fanout)
	text := string(data)
	if strings.Contains(text, "rag_merge") || strings.Contains(text, "$from(rag_merge)") {
		t.Error("request-fanout.yaml still references rag_merge (GH-365)")
	}
	for _, sel := range []string{
		"$from(rag_queries).items",
		"result.structured_output.mapped.embedding_model",
		"$from(partition_embedding_models).matched",
		"result.structured_output.mapped.documents",
		"query_failed: $from(partition_query_results).unmatched",
		"\"embedding_model_excluded\": {{ json model_excluded }}",
	} {
		if !strings.Contains(text, sel) {
			t.Errorf("source-count-independent fan-out is missing %s", sel)
		}
	}
	// The base declarations must no longer carry the fan-out words.
	base := chatbotAssetPath("request-declarations.yaml")
	bdata := readRequiredChatbotAsset(t, base)
	if strings.Contains(string(bdata), "rag_merge") {
		t.Error("request-declarations.yaml still references rag_merge (GH-365)")
	}
	if strings.Contains(string(bdata), "name: rag_query") {
		t.Error("request-declarations.yaml still declares the fan-out words")
	}
}
