// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// perDeclaredModelFloor is the volume a single declared model is allowed to
// need. The largest model the release names is 5.6GB on disk, and a blob
// downloads beside the one being written, so eight leaves room for both.
const perDeclaredModelFloor = 8

// The GKE overlay must claim enough for the models the release declares.
// On kind the models arrive from a seeded host cache, so the chart default
// is never asked to hold all of them; on GKE they are pulled, the volume
// fills mid-pull, and the preload Job dies with every rag agent waiting on
// it forever (GH-2453). A fourth model must raise the claim, not the
// operator's debugging time.
func TestGcpOverlayClaimsEnoughForTheDeclaredModels(t *testing.T) {
	root := repositoryChartRoot(t)
	models := declaredModelCount(t, filepath.Join(root, "values.yaml"))
	claimed := claimedOllamaGi(t, filepath.Join(root, "ci", gcpValuesFile))

	if want := models * perDeclaredModelFloor; claimed < want {
		t.Fatalf("the GKE overlay claims %dGi for %d declared models; %dGi is the floor",
			claimed, models, want)
	}
}

// The kind path is untouched: its overlay leaves the chart default alone,
// because a seeded cache is what fills that volume.
func TestKindOverlayDoesNotResizeTheModelVolume(t *testing.T) {
	root := repositoryChartRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "ci", chatbotDemoValuesFile))
	if err != nil {
		t.Fatalf("read the kind overlay: %v", err)
	}
	var overlay struct {
		Ollama struct {
			Persistence struct {
				Size string `yaml:"size"`
			} `yaml:"persistence"`
		} `yaml:"ollama"`
	}
	if err := yaml.Unmarshal(data, &overlay); err != nil {
		t.Fatalf("parse the kind overlay: %v", err)
	}
	if overlay.Ollama.Persistence.Size != "" {
		t.Errorf("the kind overlay now resizes the model volume to %q",
			overlay.Ollama.Persistence.Size)
	}
}

func repositoryChartRoot(t *testing.T) string {
	t.Helper()
	working, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	return filepath.Join(working, "..", "helm")
}

func declaredModelCount(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read chart values: %v", err)
	}
	var values struct {
		Ollama struct {
			Models struct {
				Embedding string   `yaml:"embedding"`
				Chat      []string `yaml:"chat"`
				Tier      string   `yaml:"tier"`
			} `yaml:"models"`
		} `yaml:"ollama"`
	}
	if err := yaml.Unmarshal(data, &values); err != nil {
		t.Fatalf("parse chart values: %v", err)
	}
	declared := map[string]bool{}
	for _, model := range append([]string{
		values.Ollama.Models.Embedding, values.Ollama.Models.Tier,
	}, values.Ollama.Models.Chat...) {
		if model != "" {
			declared[model] = true
		}
	}
	if len(declared) == 0 {
		t.Fatal("the release declares no models, so the floor cannot be computed")
	}
	return len(declared)
}

func claimedOllamaGi(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the GKE overlay: %v", err)
	}
	var overlay struct {
		Ollama struct {
			Persistence struct {
				Size string `yaml:"size"`
			} `yaml:"persistence"`
		} `yaml:"ollama"`
	}
	if err := yaml.Unmarshal(data, &overlay); err != nil {
		t.Fatalf("parse the GKE overlay: %v", err)
	}
	size := overlay.Ollama.Persistence.Size
	if size == "" {
		t.Fatal("the GKE overlay claims no model volume, so it inherits the kind default")
	}
	digits := strings.TrimSuffix(size, "Gi")
	gi, err := strconv.Atoi(digits)
	if err != nil {
		t.Fatalf("the GKE overlay claim %q is not a whole number of Gi", size)
	}
	return gi
}
