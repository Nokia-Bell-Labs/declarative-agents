// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

const aggregateOllamaCacheRoot = "/var/lib/declarative-agents/ollama-cache"

type aggregateOllamaCache struct {
	HostPath string
	Identity string
	Reused   bool
}

func prepareAggregateOllamaCache(
	run helmLLMCommandRunner,
	cluster kindrig.Cluster,
	imageID string,
	models []string,
) (aggregateOllamaCache, error) {
	if activeIntegrationKindSession() == nil ||
		(!cluster.Created && !aggregateKindClusterOwned(cluster.Name)) {
		return aggregateOllamaCache{}, nil
	}
	identity, err := ollamaCacheIdentity(imageID, models)
	if err != nil {
		return aggregateOllamaCache{}, err
	}
	cache := aggregateOllamaCache{
		HostPath: filepath.Join(aggregateOllamaCacheRoot, identity),
		Identity: identity,
	}
	node := cluster.Name + "-control-plane"
	check := `test -f "$1/ready" && cat "$1/identity"`
	output, checkErr := run(
		"docker", "exec", node, "sh", "-c", check, "--", cache.HostPath)
	if checkErr == nil && strings.TrimSpace(string(output)) == identity {
		cache.Reused = true
		resetActive := `rm -rf "$1/active" && mkdir -p "$1/active"`
		if activeOutput, activeErr := run(
			"docker", "exec", node, "sh", "-c", resetActive, "--", cache.HostPath,
		); activeErr != nil {
			return aggregateOllamaCache{}, fmt.Errorf(
				"reset aggregate Ollama active storage %s: %w: %s",
				cache.HostPath, activeErr, strings.TrimSpace(string(activeOutput)))
		}
	} else {
		reset := `rm -rf "$1" && mkdir -p "$1/models" "$1/active" && printf '%s\n' "$2" > "$1/identity"`
		if resetOutput, resetErr := run(
			"docker", "exec", node, "sh", "-c", reset, "--", cache.HostPath, identity,
		); resetErr != nil {
			return aggregateOllamaCache{}, fmt.Errorf(
				"prepare aggregate Ollama cache %s: %w: %s",
				cache.HostPath, resetErr, strings.TrimSpace(string(resetOutput)))
		}
	}
	if !registerAggregateFinalizer("ollama-model-cache", func() error {
		output, err := run(
			"docker", "exec", node, "rm", "-rf", aggregateOllamaCacheRoot)
		if err != nil {
			return fmt.Errorf("remove aggregate Ollama cache: %w: %s",
				err, strings.TrimSpace(string(output)))
		}
		return nil
	}) {
		return aggregateOllamaCache{}, fmt.Errorf(
			"aggregate Ollama cache prepared without an active session")
	}
	outcome := "empty"
	if cache.Reused {
		outcome = "reused"
	}
	fmt.Printf("helmLLMTier: aggregate model cache %s identity=%s path=%s\n",
		outcome, identity, cache.HostPath)
	return cache, nil
}

func ollamaCacheIdentity(imageID string, models []string) (string, error) {
	imageID = strings.TrimSpace(imageID)
	if !strings.HasPrefix(imageID, "sha256:") {
		return "", fmt.Errorf("Ollama cache image ID %q is not a verified digest", imageID)
	}
	unique := make(map[string]struct{}, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			return "", fmt.Errorf("Ollama cache model set contains an empty model")
		}
		unique[model] = struct{}{}
	}
	if len(unique) == 0 {
		return "", fmt.Errorf("Ollama cache model set is empty")
	}
	canonical := make([]string, 0, len(unique))
	for model := range unique {
		canonical = append(canonical, model)
	}
	sort.Strings(canonical)
	sum := sha256.Sum256([]byte(strings.Join(append(
		[]string{"ollama-cache/v1", imageID}, canonical...), "\x00")))
	return fmt.Sprintf("%x", sum), nil
}
