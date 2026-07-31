// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfiguredOllamaModelFollowsShippedProfile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
		want    string
		wantErr bool
	}{
		{name: "Qwen variant", content: "tools:\n  - name: invoke_llm\n    config:\n      model: qwen3.6:35b-mlx\n", want: "qwen3.6:35b-mlx"},
		{name: "alternate variant", content: "tools:\n  - name: invoke_llm\n    config:\n      model: qwen3:8b\n", want: "qwen3:8b"},
		{name: "missing model", content: "tools:\n  - name: invoke_llm\n    config: {}\n", wantErr: true},
		{name: "schema placeholder before config", content: "tools:\n  - name: invoke_llm\n    requirements:\n      request_shape:\n        model: {type: string}\n    config:\n      model: qwen3.6:35b-mlx\n", want: "qwen3.6:35b-mlx"},
		{name: "schema placeholder only", content: "tools:\n  - name: invoke_llm\n    requirements:\n      request_shape:\n        model: {type: string}\n", wantErr: true},
		{name: "wrong tool", content: "tools:\n  - name: other\n    config:\n      model: unrelated:9b\n", wantErr: true},
		{name: "malformed YAML", content: "tools: [", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			path := coreIntegrationProfilePath(root, ollamaLLMRel)
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatalf("create fixture directory: %v", err)
			}
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			got, err := configuredOllamaModelFromRoot(root)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("configuredOllamaModelFromRoot() = %q, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("configuredOllamaModelFromRoot() error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("configuredOllamaModelFromRoot() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestListedSummaryModelsToleratesProseConnectives(t *testing.T) {
	summary := "Local Ollama models include: ornith:9b, qwen3.6:35b-mlx and glm-5.2:cloud."
	got := listedSummaryModels(summary)
	want := []string{"ornith:9b", "qwen3.6:35b-mlx", "glm-5.2:cloud"}
	if len(got) != len(want) {
		t.Fatalf("listedSummaryModels() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("listedSummaryModels()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
