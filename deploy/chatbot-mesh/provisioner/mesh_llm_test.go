// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"strings"
	"testing"
)

func inClusterMesh() MeshView {
	return MeshView{
		Rags:   []RagView{{Name: "rag0", Collection: "corpus", EmbeddingModel: "all-minilm", Replicas: 1}},
		LLM:    LLMView{InCluster: true, EmbedModel: "all-minilm", ChatModels: []string{"qwen2.5:0.5b", "ornith:9b"}, RouterModel: "qwen2.5:0.5b", Topology: "single"},
		Params: ParamsView{NResults: 5},
	}
}

func TestValidateInClusterRequiresModels(t *testing.T) {
	m := inClusterMesh()
	if err := m.Validate(); err != nil {
		t.Fatalf("valid in-cluster mesh rejected: %v", err)
	}
	// Missing chat models is rejected.
	bad := inClusterMesh()
	bad.LLM.ChatModels = nil
	if err := bad.Validate(); err == nil {
		t.Fatal("in-cluster mesh with no chat model must be rejected")
	}
	// Bad topology is rejected.
	bad2 := inClusterMesh()
	bad2.LLM.Topology = "cluster"
	if err := bad2.Validate(); err == nil {
		t.Fatal("invalid topology must be rejected")
	}
}

func TestHelmSetArgsRendersOllamaModels(t *testing.T) {
	args := strings.Join(inClusterMesh().HelmSetArgs(), " ")
	for _, want := range []string{
		"ollama.enabled=true",
		"ollama.models.embedding=all-minilm",
		"ollama.models.router=qwen2.5:0.5b",
		"ollama.models.chat[0]=qwen2.5:0.5b",
		"ollama.models.chat[1]=ornith:9b",
		"ollama.topology=single",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("helm set args missing %q; got %s", want, args)
		}
	}
}

func TestHelmSetArgsExternalOmitsOllamaModels(t *testing.T) {
	m := inClusterMesh()
	m.LLM.InCluster = false
	m.LLM.ExternalURL = "http://o:11434"
	args := strings.Join(m.HelmSetArgs(), " ")
	if strings.Contains(args, "ollama.models") {
		t.Errorf("external mesh must not render ollama.models; got %s", args)
	}
	if !strings.Contains(args, "ollama.enabled=false") {
		t.Errorf("external mesh must set ollama.enabled=false; got %s", args)
	}
}
