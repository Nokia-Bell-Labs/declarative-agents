// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCodingAgentImageBuildUsesOnlyCanonicalRecipe(t *testing.T) {
	coreRoot := codingAgentCoreRoot(filepath.Join(string(filepath.Separator), "repo", "applications", "coding-agent"))
	if coreRoot != filepath.Join(string(filepath.Separator), "repo", "agent-core") {
		t.Fatalf("agent-core root = %q", coreRoot)
	}
	if _, err := os.Stat(filepath.Join("..", "..", "..", "agent-core", "Dockerfile")); err != nil {
		t.Fatalf("canonical agent-core Dockerfile: %v", err)
	}
	if _, err := os.Stat(filepath.Join("..", "..", "..", "agent-core", "toolchain.Dockerfile")); !os.IsNotExist(err) {
		t.Fatalf("alternate toolchain Dockerfile still exists: %v", err)
	}
}

func TestChartDefaultsToCanonicalAgentCoreImage(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "helm", "values.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var values struct {
		Image struct {
			Repository string `yaml:"repository"`
			Tag        string `yaml:"tag"`
		} `yaml:"image"`
	}
	if err := yaml.Unmarshal(data, &values); err != nil {
		t.Fatal(err)
	}
	if values.Image.Repository != codingAgentImageRepository ||
		values.Image.Tag != codingAgentImageTag {
		t.Fatalf("default image = %s:%s, want %s:%s",
			values.Image.Repository, values.Image.Tag,
			codingAgentImageRepository, codingAgentImageTag)
	}
	if !strings.HasSuffix(values.Image.Repository, "/agent-core") {
		t.Fatal("chart does not use the canonical agent-core image")
	}
}
