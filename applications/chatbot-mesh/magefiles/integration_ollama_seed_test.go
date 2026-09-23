// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
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
	platform := kindrig.HostPlatform()
	seedImage, err := kindrig.FormatRecipeLocal(
		kindrig.CacheRole, "ollama-models", "sha256:"+strings.Repeat("a", 64), platform)
	if err != nil {
		t.Fatal(err)
	}
	identity := ollamaSeedIdentity{
		recipe:    "sha256:recipe",
		runtimeID: "sha256:runtime",
		platform:  platform,
	}
	args := strings.Join(ollamaSeedBuildArgs(
		seedImage,
		"localhost/declarative-agents/derived/ollama:upstream-0.34.2-kind-trusted-recipe-0123456789ab-linux-amd64",
		"all-minilm qwen2.5:0.5b",
		identity,
	), " ")
	for _, want := range []string{
		"--provenance=false",
		"RUNTIME_IMAGE=localhost/declarative-agents/derived/ollama:upstream-0.34.2-kind-trusted-recipe-0123456789ab-linux-amd64",
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
		seedImage, payload, identity)
	if !matches || result.ImageID != "sha256:seed" {
		t.Fatalf("matching seed rejected: result=%+v matches=%v", result, matches)
	}
	stale := identity
	stale.runtimeID = "sha256:stale"
	if _, matches := ollamaSeedInspectPayload(
		seedImage, payload, stale,
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
