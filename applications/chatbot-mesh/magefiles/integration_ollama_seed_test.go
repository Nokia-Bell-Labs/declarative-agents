// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

func TestOllamaSeedRecipeKeysRuntimeAndModels(t *testing.T) {
	platform := "linux/" + runtime.GOARCH
	runtimeID := "sha256:" + strings.Repeat("a", 64)
	first := ollamaSeedRecipe(
		"ollama:trusted", runtimeID, platform, []string{"chat", "embed"})
	reordered := ollamaSeedRecipe(
		"ollama:trusted", runtimeID, platform, []string{"chat", "embed"})
	changedModel := ollamaSeedRecipe(
		"ollama:trusted", runtimeID, platform, []string{"chat", "other"})
	changedRuntime := ollamaSeedRecipe(
		"ollama:trusted", "sha256:"+strings.Repeat("b", 64),
		platform, []string{"chat", "embed"})
	if first != reordered {
		t.Fatal("identical seed inputs produced different recipes")
	}
	if first == changedModel || first == changedRuntime {
		t.Fatal("seed recipe did not change with model or runtime identity")
	}
}

func TestOllamaSeedBuildAndInspectionRequireExactIdentity(t *testing.T) {
	identity := ollamaSeedIdentity{
		recipe:    "sha256:recipe",
		runtimeID: "sha256:runtime",
		platform:  "linux/" + runtime.GOARCH,
	}
	args := strings.Join(ollamaSeedBuildArgs(
		ollamaSeedRepository+":test",
		"declarative-agents/ollama:trusted",
		"all-minilm qwen2.5:0.5b",
		identity,
	), " ")
	for _, want := range []string{
		"--provenance=false",
		"RUNTIME_IMAGE=declarative-agents/ollama:trusted",
		"MODELS=all-minilm qwen2.5:0.5b",
		ollamaSeedRecipeLabel + "=" + identity.recipe,
		ollamaSeedRuntimeLabel + "=" + identity.runtimeID,
		ollamaSeedPlatformLabel + "=" + identity.platform,
	} {
		if !strings.Contains(args, want) {
			t.Errorf("seed build args missing %q: %s", want, args)
		}
	}
	payload, _ := json.Marshal([]map[string]any{{
		"Id": "sha256:seed", "Os": "linux", "Architecture": runtime.GOARCH,
		"Config": map[string]any{"Labels": map[string]string{
			ollamaSeedRecipeLabel:   identity.recipe,
			ollamaSeedRuntimeLabel:  identity.runtimeID,
			ollamaSeedPlatformLabel: identity.platform,
		}},
	}})
	result, matches := ollamaSeedInspectPayload(
		ollamaSeedRepository+":test", payload, identity)
	if !matches || result.ImageID != "sha256:seed" {
		t.Fatalf("matching seed rejected: result=%+v matches=%v", result, matches)
	}
	stale := identity
	stale.runtimeID = "sha256:stale"
	if _, matches := ollamaSeedInspectPayload(
		ollamaSeedRepository+":test", payload, stale,
	); matches {
		t.Fatal("stale runtime identity reused seed image")
	}
}

func TestOllamaSeedDockerfilePullsCanonicalModelsIntoImage(t *testing.T) {
	dockerfile := ollamaSeedDockerfile()
	for _, want := range []string{
		"ENV OLLAMA_MODELS=/opt/ollama-seed",
		"ollama serve",
		"for model in $MODELS",
		`ollama pull "$model"`,
	} {
		if !strings.Contains(dockerfile, want) {
			t.Errorf("seed Dockerfile missing %q:\n%s", want, dockerfile)
		}
	}
}

func TestOllamaSeedTransferSkipsReadyAggregateCache(t *testing.T) {
	err := seedAggregateOllamaCache(
		ollamaSeedImage{Reference: "invalid.example/unused:seed"},
		"unused-cluster",
		aggregateOllamaCache{
			HostPath: aggregateOllamaCacheRoot + "/" + strings.Repeat("a", 64),
			Reused:   true,
		},
	)
	if err != nil {
		t.Fatalf("ready cache attempted seed transfer: %v", err)
	}
}
