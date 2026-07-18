// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfiguredChromaModelsDefault(t *testing.T) {
	t.Setenv(chromaChatModelEnv, "")
	t.Setenv(chromaEmbedModelEnv, "")
	if got := configuredChromaChatModel(); got != chromaChatModel {
		t.Fatalf("chat model default = %q, want %q", got, chromaChatModel)
	}
	if got := configuredChromaEmbedModel(); got != chromaEmbedModel {
		t.Fatalf("embed model default = %q, want %q", got, chromaEmbedModel)
	}
}

func TestConfiguredChromaModelsOverride(t *testing.T) {
	t.Setenv(chromaChatModelEnv, "custom-chat")
	t.Setenv(chromaEmbedModelEnv, "custom-embed")
	if got := configuredChromaChatModel(); got != "custom-chat" {
		t.Fatalf("chat model override = %q, want custom-chat", got)
	}
	if got := configuredChromaEmbedModel(); got != "custom-embed" {
		t.Fatalf("embed model override = %q, want custom-embed", got)
	}
}

func TestChromaModelInstalledTagTolerance(t *testing.T) {
	names := []string{"all-minilm:latest", "qwen3.6:35b-mlx"}
	for _, model := range []string{"all-minilm", "all-minilm:latest", "qwen3.6:35b-mlx"} {
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
		spanLine("2026-07-18T02:00:00.100000000Z", "execute_tool chroma_ready", "chroma_ready", ""),
		spanLine("2026-07-18T02:00:00.400000000Z", "execute_tool embed_query", "embed_query", ""),
		spanLine("2026-07-18T02:00:00.700000000Z", "execute_tool chroma_query", "chroma_query", ""),
		spanLine("2026-07-18T02:00:01.000000000Z", "chat qwen3.6:35b-mlx", "invoke_llm", ""),
		spanLine("2026-07-18T02:00:01.300000000Z", "agent.loop.done", "done", "The corpus describes specification-driven development."),
	})
	if err := assertChromaReaderTrace(trace); err != nil {
		t.Fatalf("expected reader trace to pass, got %v", err)
	}
}

func TestAssertChromaReaderTraceRejectsBadOrder(t *testing.T) {
	trace := writeChromaTrace(t, []string{
		spanLine("2026-07-18T02:00:00.400000000Z", "execute_tool chroma_query", "chroma_query", ""),
		spanLine("2026-07-18T02:00:00.700000000Z", "execute_tool embed_query", "embed_query", ""),
		spanLine("2026-07-18T02:00:01.000000000Z", "chat qwen3.6:35b-mlx", "invoke_llm", ""),
		spanLine("2026-07-18T02:00:01.300000000Z", "agent.loop.done", "done", "answer"),
	})
	if err := assertChromaReaderTrace(trace); err == nil {
		t.Fatal("expected reader trace with reversed embed/query order to fail")
	}
}

func TestAssertChromaReaderTraceRequiresAnswer(t *testing.T) {
	trace := writeChromaTrace(t, []string{
		spanLine("2026-07-18T02:00:00.400000000Z", "execute_tool embed_query", "embed_query", ""),
		spanLine("2026-07-18T02:00:00.700000000Z", "execute_tool chroma_query", "chroma_query", ""),
		spanLine("2026-07-18T02:00:01.000000000Z", "chat qwen3.6:35b-mlx", "invoke_llm", ""),
	})
	if err := assertChromaReaderTrace(trace); err == nil {
		t.Fatal("expected reader trace without done.summary to fail")
	}
}

func TestAssertChromaIngestTrace(t *testing.T) {
	trace := writeChromaTrace(t, []string{
		spanLine("2026-07-18T02:00:00.100000000Z", "execute_tool chroma_ready", "chroma_ready", ""),
		spanLine("2026-07-18T02:00:00.200000000Z", "execute_tool ollama_ready", "ollama_ready", ""),
		spanLine("2026-07-18T02:00:00.900000000Z", "execute_tool chroma_count", "chroma_count", ""),
	})
	if err := assertChromaIngestTrace(trace); err != nil {
		t.Fatalf("expected ingest trace to pass, got %v", err)
	}
}

func TestAssertChromaIngestTraceMissingWord(t *testing.T) {
	trace := writeChromaTrace(t, []string{
		spanLine("2026-07-18T02:00:00.100000000Z", "execute_tool chroma_ready", "chroma_ready", ""),
		spanLine("2026-07-18T02:00:00.200000000Z", "execute_tool ollama_ready", "ollama_ready", ""),
	})
	if err := assertChromaIngestTrace(trace); err == nil {
		t.Fatal("expected ingest trace without chroma_count to fail")
	}
}

// spanLine renders one stdouttrace-style ndjson span with a command.name and an
// optional done.summary attribute.
func spanLine(start, name, commandName, summary string) string {
	attrs := `{"Key":"command.name","Value":{"Type":"STRING","Value":"` + commandName + `"}}`
	if summary != "" {
		attrs += `,{"Key":"done.summary","Value":{"Type":"STRING","Value":"` + summary + `"}}`
	}
	return `{"Name":"` + name + `","StartTime":"` + start + `","Attributes":[` + attrs + `]}`
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
