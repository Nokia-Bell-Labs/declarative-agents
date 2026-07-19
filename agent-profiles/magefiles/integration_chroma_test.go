// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestChromaRequiredModelsFromConfig(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "agents", "knowledge-manager")
	writeChromaConfigFile(t, filepath.Join(base, "corpus-rest.yaml"),
		"rest:\n  clients:\n    ollama:\n      operations:\n        embed:\n          body:\n            model: embed-model\n")
	decl := "tools:\n  - name: read_resource\n  - name: invoke_llm\n    config:\n      model: chat-model\n"
	writeChromaConfigFile(t, filepath.Join(base, "corpus-ingest", "declarations.yaml"), decl)
	writeChromaConfigFile(t, filepath.Join(base, "corpus-reader", "declarations.yaml"), decl)

	got, err := chromaRequiredModels(root)
	if err != nil {
		t.Fatalf("chromaRequiredModels: %v", err)
	}
	want := []string{"chat-model", "embed-model"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("required models = %v, want %v (distinct, sorted)", got, want)
	}
}

func TestChromaRequiredModelsMissingInvokeLLM(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "agents", "knowledge-manager")
	writeChromaConfigFile(t, filepath.Join(base, "corpus-rest.yaml"),
		"rest:\n  clients:\n    ollama:\n      operations:\n        embed:\n          body:\n            model: embed-model\n")
	writeChromaConfigFile(t, filepath.Join(base, "corpus-ingest", "declarations.yaml"), "tools:\n  - name: read_resource\n")
	if _, err := chromaRequiredModels(root); err == nil {
		t.Fatal("expected an error when a profile has no invoke_llm model")
	}
}

func writeChromaConfigFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestChromaModelInstalledTagTolerance(t *testing.T) {
	names := []string{"qwen3-embedding:8b:latest", "ornith:9b"}
	for _, model := range []string{"qwen3-embedding:8b", "qwen3-embedding:8b:latest", "ornith:9b"} {
		if !chromaModelInstalled(names, model) {
			t.Errorf("model %q should be reported installed against %v", model, names)
		}
	}
	if chromaModelInstalled(names, "nomic-embed-text") {
		t.Errorf("absent model should not be reported installed")
	}
}

func TestIsChromaSubsequence(t *testing.T) {
	want := []string{"embed_query", "chroma_query", "invoke_llm"}
	cases := []struct {
		name string
		got  []string
		ok   bool
	}{
		{"exact", want, true},
		{"interleaved", []string{"chroma_ready", "embed_query", "resolve_collection", "chroma_query", "invoke_llm"}, true},
		{"out_of_order", []string{"chroma_query", "embed_query", "invoke_llm"}, false},
		{"missing_tail", []string{"embed_query", "chroma_query"}, false},
		{"empty", nil, false},
	}
	for _, tc := range cases {
		if got := isChromaSubsequence(want, tc.got); got != tc.ok {
			t.Errorf("%s: isChromaSubsequence(%v) = %v, want %v", tc.name, tc.got, got, tc.ok)
		}
	}
}

func TestAssertChromaReaderTraceOrder(t *testing.T) {
	trace := writeChromaTrace(t, []string{
		spanLine("2026-07-18T02:00:00.100000000Z", "execute_tool chroma_ready", "chroma_ready"),
		spanLine("2026-07-18T02:00:00.400000000Z", "execute_tool embed_query", "embed_query"),
		spanLine("2026-07-18T02:00:00.700000000Z", "execute_tool chroma_query", "chroma_query"),
		llmSpanLine("2026-07-18T02:00:01.000000000Z", 356),
	})
	if err := assertChromaReaderTrace(trace); err != nil {
		t.Fatalf("expected reader trace to pass, got %v", err)
	}
}

func TestAssertChromaReaderTraceRejectsBadOrder(t *testing.T) {
	trace := writeChromaTrace(t, []string{
		spanLine("2026-07-18T02:00:00.400000000Z", "execute_tool chroma_query", "chroma_query"),
		spanLine("2026-07-18T02:00:00.700000000Z", "execute_tool embed_query", "embed_query"),
		llmSpanLine("2026-07-18T02:00:01.000000000Z", 356),
	})
	if err := assertChromaReaderTrace(trace); err == nil {
		t.Fatal("expected reader trace with reversed embed/query order to fail")
	}
}

func TestAssertChromaReaderTraceRequiresAnswer(t *testing.T) {
	trace := writeChromaTrace(t, []string{
		spanLine("2026-07-18T02:00:00.400000000Z", "execute_tool embed_query", "embed_query"),
		spanLine("2026-07-18T02:00:00.700000000Z", "execute_tool chroma_query", "chroma_query"),
		llmSpanLine("2026-07-18T02:00:01.000000000Z", 0),
	})
	if err := assertChromaReaderTrace(trace); err == nil {
		t.Fatal("expected reader trace with no invoke_llm output tokens to fail")
	}
}

func TestAssertChromaIngestTrace(t *testing.T) {
	trace := writeChromaTrace(t, []string{
		spanLine("2026-07-18T02:00:00.100000000Z", "execute_tool chroma_ready", "chroma_ready"),
		spanLine("2026-07-18T02:00:00.200000000Z", "execute_tool ollama_ready", "ollama_ready"),
		spanLine("2026-07-18T02:00:00.900000000Z", "execute_tool chroma_count", "chroma_count"),
	})
	if err := assertChromaIngestTrace(trace); err != nil {
		t.Fatalf("expected ingest trace to pass, got %v", err)
	}
}

func TestAssertChromaIngestTraceMissingWord(t *testing.T) {
	trace := writeChromaTrace(t, []string{
		spanLine("2026-07-18T02:00:00.100000000Z", "execute_tool chroma_ready", "chroma_ready"),
		spanLine("2026-07-18T02:00:00.200000000Z", "execute_tool ollama_ready", "ollama_ready"),
	})
	if err := assertChromaIngestTrace(trace); err == nil {
		t.Fatal("expected ingest trace without chroma_count to fail")
	}
}

// spanLine renders one stdouttrace-style ndjson span carrying a command.name.
func spanLine(start, name, commandName string) string {
	attrs := `{"Key":"command.name","Value":{"Type":"STRING","Value":"` + commandName + `"}}`
	return `{"Name":"` + name + `","StartTime":"` + start + `","Attributes":[` + attrs + `]}`
}

// llmSpanLine renders an invoke_llm dispatch span with an output-token count,
// matching the inference span the reader emits for its grounded answer.
func llmSpanLine(start string, outputTokens int) string {
	attrs := `{"Key":"command.name","Value":{"Type":"STRING","Value":"invoke_llm"}},` +
		`{"Key":"gen_ai.usage.output_tokens","Value":{"Type":"INT64","Value":` + strconv.Itoa(outputTokens) + `}}`
	return `{"Name":"chat qwen3.5:9b","StartTime":"` + start + `","Attributes":[` + attrs + `]}`
}

func writeChromaTrace(t *testing.T, lines []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "trace.ndjson")
	content := ""
	for _, line := range lines {
		content += line + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write trace: %v", err)
	}
	return path
}
