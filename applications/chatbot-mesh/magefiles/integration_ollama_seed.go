// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	ollamaSeedRepository    = "declarative-agents/ollama-model-cache"
	ollamaSeedRecipeLabel   = "io.declarative-agents.ollama-seed.recipe"
	ollamaSeedRuntimeLabel  = "io.declarative-agents.ollama-seed.runtime"
	ollamaSeedPlatformLabel = "io.declarative-agents.ollama-seed.platform"
)

type ollamaSeedImage struct {
	Reference string
	ImageID   string
	Recipe    string
	Reused    bool
}

func ensureOllamaSeedImage(
	runtimeImage, runtimeID string,
	models []string,
) (ollamaSeedImage, error) {
	canonical, err := canonicalOllamaModels(models)
	if err != nil {
		return ollamaSeedImage{}, err
	}
	if !strings.HasPrefix(runtimeID, "sha256:") {
		return ollamaSeedImage{}, fmt.Errorf(
			"Ollama seed runtime ID %q is not a verified digest", runtimeID)
	}
	platform := "linux/" + runtime.GOARCH
	recipe := ollamaSeedRecipe(runtimeImage, runtimeID, platform, canonical)
	image := ollamaSeedRepository + ":" + strings.TrimPrefix(recipe, "sha256:")[:12]
	identity := ollamaSeedIdentity{
		recipe: recipe, runtimeID: runtimeID, platform: platform,
	}
	if result, matches := inspectOllamaSeedImage(image, identity); matches {
		result.Reused = true
		fmt.Printf("helmLLMTier: reusing model seed image %s digest=%s\n",
			result.Reference, result.ImageID)
		return result, nil
	}

	contextDir, err := os.MkdirTemp("", "chatbot-mesh-ollama-seed-*")
	if err != nil {
		return ollamaSeedImage{}, err
	}
	defer func() { _ = os.RemoveAll(contextDir) }()
	if err := os.WriteFile(
		filepath.Join(contextDir, "Dockerfile"),
		[]byte(ollamaSeedDockerfile()), 0o644,
	); err != nil {
		return ollamaSeedImage{}, fmt.Errorf("write Ollama seed Dockerfile: %w", err)
	}
	started := time.Now()
	args := ollamaSeedBuildArgs(
		image, runtimeImage, strings.Join(canonical, " "), identity)
	command := exec.Command("docker", args...)
	command.Dir = contextDir
	command.Stdout, command.Stderr = os.Stderr, os.Stderr
	if err := command.Run(); err != nil {
		return ollamaSeedImage{}, fmt.Errorf("build Ollama model seed %s: %w", image, err)
	}
	result, matches := inspectOllamaSeedImage(image, identity)
	if !matches {
		return ollamaSeedImage{}, fmt.Errorf(
			"built Ollama model seed %s does not carry its verified identity", image)
	}
	fmt.Printf("helmLLMTier: built model seed image %s digest=%s elapsed=%s\n",
		result.Reference, result.ImageID, time.Since(started).Round(time.Millisecond))
	return result, nil
}

func ollamaSeedRecipe(
	runtimeImage, runtimeID, platform string,
	canonical []string,
) string {
	sum := sha256.Sum256([]byte(strings.Join(append([]string{
		"ollama-seed-image/v1",
		ollamaSeedDockerfile(),
		runtimeImage,
		runtimeID,
		platform,
	}, canonical...), "\x00")))
	return fmt.Sprintf("sha256:%x", sum)
}

type ollamaSeedIdentity struct {
	recipe    string
	runtimeID string
	platform  string
}

func ollamaSeedBuildArgs(
	image, runtimeImage, models string,
	identity ollamaSeedIdentity,
) []string {
	return []string{
		"build", "--platform", identity.platform, "--provenance=false",
		"--build-arg", "RUNTIME_IMAGE=" + runtimeImage,
		"--build-arg", "MODELS=" + models,
		"--label", ollamaSeedRecipeLabel + "=" + identity.recipe,
		"--label", ollamaSeedRuntimeLabel + "=" + identity.runtimeID,
		"--label", ollamaSeedPlatformLabel + "=" + identity.platform,
		"-t", image, ".",
	}
}

func inspectOllamaSeedImage(
	image string,
	identity ollamaSeedIdentity,
) (ollamaSeedImage, bool) {
	output, err := exec.Command("docker", "image", "inspect", image).Output()
	if err != nil {
		return ollamaSeedImage{}, false
	}
	return ollamaSeedInspectPayload(image, output, identity)
}

func ollamaSeedInspectPayload(
	image string,
	output []byte,
	identity ollamaSeedIdentity,
) (ollamaSeedImage, bool) {
	var inspected []struct {
		ID           string
		Os           string
		Architecture string
		Config       struct {
			Labels map[string]string
		}
	}
	if json.Unmarshal(output, &inspected) != nil || len(inspected) != 1 {
		return ollamaSeedImage{}, false
	}
	item := inspected[0]
	matches := strings.HasPrefix(item.ID, "sha256:") &&
		item.Os+"/"+item.Architecture == identity.platform &&
		item.Config.Labels[ollamaSeedRecipeLabel] == identity.recipe &&
		item.Config.Labels[ollamaSeedRuntimeLabel] == identity.runtimeID &&
		item.Config.Labels[ollamaSeedPlatformLabel] == identity.platform
	return ollamaSeedImage{
		Reference: image,
		ImageID:   item.ID,
		Recipe:    identity.recipe,
	}, matches
}

func ollamaSeedDockerfile() string {
	return `ARG RUNTIME_IMAGE
FROM ${RUNTIME_IMAGE}
ARG MODELS
ENV OLLAMA_MODELS=/opt/ollama-seed OLLAMA_HOST=127.0.0.1:11434
RUN set -eu; \
    ollama serve >/tmp/ollama-seed.log 2>&1 & pid=$!; \
    trap 'kill "$pid" 2>/dev/null || true' EXIT; \
    until ollama list >/dev/null 2>&1; do sleep 1; done; \
    for model in $MODELS; do ollama pull "$model"; done; \
    kill "$pid"; wait "$pid" || true; trap - EXIT
`
}

func seedAggregateOllamaCache(
	seed ollamaSeedImage,
	cluster string,
	cache aggregateOllamaCache,
) error {
	if cache.HostPath == "" || cache.Reused {
		return nil
	}
	createOutput, err := exec.Command("docker", "create", seed.Reference).CombinedOutput()
	if err != nil {
		return fmt.Errorf("create Ollama seed container: %w: %s",
			err, strings.TrimSpace(string(createOutput)))
	}
	container := strings.TrimSpace(string(createOutput))
	defer func() {
		_ = exec.Command("docker", "rm", "-f", container).Run()
	}()

	source := exec.Command(
		"docker", "cp", container+":/opt/ollama-seed/.", "-")
	stream, err := source.StdoutPipe()
	if err != nil {
		return err
	}
	node := cluster + "-control-plane"
	target := exec.Command(
		"docker", "exec", "-i", node,
		"tar", "-C", cache.HostPath+"/models", "-xf", "-")
	target.Stdin = stream
	var targetOutput bytes.Buffer
	target.Stdout, target.Stderr = &targetOutput, &targetOutput
	if err := target.Start(); err != nil {
		return fmt.Errorf("start Ollama seed extraction: %w", err)
	}
	if err := source.Run(); err != nil {
		if target.Process != nil {
			_ = target.Process.Kill()
		}
		_ = target.Wait()
		return fmt.Errorf("stream Ollama seed image: %w", err)
	}
	if err := target.Wait(); err != nil {
		return fmt.Errorf("extract Ollama seed into aggregate cache: %w: %s",
			err, strings.TrimSpace(targetOutput.String()))
	}
	markerOutput, err := exec.Command(
		"docker", "exec", node, "touch", cache.HostPath+"/seeded").CombinedOutput()
	if err != nil {
		return fmt.Errorf("mark aggregate Ollama cache seeded: %w: %s",
			err, strings.TrimSpace(string(markerOutput)))
	}
	return nil
}
