// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCodingAgentImageBuildUsesPublishedRecipe(t *testing.T) {
	applicationRoot := filepath.Join(string(filepath.Separator), "repo", "examples", "coding-agent")
	contextDir, dockerfile, args := codingAgentImageBuild(applicationRoot, "example/runtime:test")
	if contextDir != filepath.Join(string(filepath.Separator), "repo") {
		t.Errorf("build context = %q, want repository root", contextDir)
	}
	if dockerfile != filepath.Join(applicationRoot, "Dockerfile") {
		t.Errorf("Dockerfile = %q", dockerfile)
	}
	want := []string{
		"build", "--pull=false",
		"--build-arg", "GOLANGCI_LINT_VERSION=" + codingAgentGolangciLint,
		"-f", dockerfile,
		"-t", "example/runtime:test",
		".",
	}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("docker args = %#v, want %#v", args, want)
	}
}

func TestChartDefaultsToCodingToolchainImage(t *testing.T) {
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
	if strings.HasSuffix(values.Image.Repository, "/agent-core") {
		t.Fatal("chart reverted to the runtime-only agent-core image")
	}
}
